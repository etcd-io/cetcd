// Copyright 2016 CoreOS, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package cetcd

import (
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"golang.org/x/net/context"
	"io/ioutil"
	"net/http"
	"net/url"
	"strings"
	"time"

	etcd "github.com/coreos/etcd/clientv3"
	v3sync "github.com/coreos/etcd/clientv3/concurrency"
	"github.com/gorilla/schema"
)

type kvHandler struct {
	cli *etcd.Client
	dec *schema.Decoder
}

// get http handler for kv
func NewKVHandler(cli *etcd.Client) http.Handler {
	return &kvHandler{cli, schema.NewDecoder()}
}

var errMissingKeyName = errors.New("Missing key name")
var errLocked = errors.New("key is locked")
var errNoLease = errors.New("no lease")
var errBadBody = errors.New("bad body")

func (kvh *kvHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var err error
	path := strings.Replace(r.URL.Path, "/v1/kv/", "", 1)
	values := r.URL.Query()
	/* this accepts ?mIxedCaseArguments, but consul expects lowercase.
	for k, v := range values {
		delete(values, k)
		values[strings.ToLower(k)] = v
	}
	*/
	switch r.Method {
	case "PUT":
		datc := make(chan string, 1)
		go func() {
			// Consul expects the request body to be key's value.
			defer close(datc)
			dat, rerr := ioutil.ReadAll(r.Body)
			if rerr != nil {
				err = rerr
			} else {
				datc <- string(dat)
			}
		}()
		v := &putRequest{path: path, datc: datc, values: values, ctx: r.Context()}
		if err = kvh.dec.Decode(v, values); err != nil {
			break
		}
		if perr := kvh.put(w, v); perr != nil && err == nil {
			err = perr
		}
	case "GET":
		v := &getRequest{path: path, values: values, ctx: r.Context()}
		if err = kvh.dec.Decode(v, values); err != nil {
			break
		}
		err = kvh.get(w, v)
	case "DELETE":
		v := &delRequest{path: path, values: values, ctx: r.Context()}
		if err = kvh.dec.Decode(v, values); err != nil {
			break
		}
		err = kvh.del(w, v)
	default:
		err = fmt.Errorf("unknown method %q", r.Method)
	}
	if err != nil {
		http.Error(w, err.Error(), 400)
	}
}

func (kvh *kvHandler) put(w http.ResponseWriter, req *putRequest) error {
	if req.path == "" {
		return errMissingKeyName
	}
	applyf := func(s v3sync.STM) error { return applyPut(s, req) }
	_, err := v3sync.NewSTMRepeatable(req.ctx, kvh.cli, applyf)
	if err != nil {
		_, err = w.Write([]byte("false"))
		return err
	}
	_, err = w.Write([]byte("true"))
	return err
}

func applyPut(s v3sync.STM, req *putRequest) (err error) {
	if _, ok := req.values["cas"]; ok && uint64(s.Rev("/con/value/"+req.path)) != req.Cas {
		if req.Cas == 0 {
			// put key if it does not exist
			return fmt.Errorf("key exists")
		}
		// set only if index matches modify index
		return fmt.Errorf("key index mismatch")
	}
	if len(req.Release) != 0 && s.Get("/con/lock/"+req.path) == req.Release {
		s.Put("/con/lock/"+req.path, "")
	}
	if len(req.Acquire) != 0 {
		lid, err := leaseIDFromSession(req.Acquire)
		if err != nil {
			return err
		}
		switch s.Get("/con/lock/" + req.path) {
		// lock is not held
		case "":
			// if session isn't valid, puts will fail, no need to check.

			// increment LockIndex and set key's session.
			s.Put("/con/lock/"+req.path, req.Acquire, etcd.WithLease(lid))
			if req.Acquire[:1] == "d" {
				// delete when session expires
				s.Put("/con/lidx/"+req.path, "", etcd.WithLease(lid))
			} else {
				s.Put("/con/lidx/"+req.path, "")
			}
		// lock already held by given session
		case req.Acquire:
			// LockIndex is unmodified; key contents are updated.
		// another session holds lock
		default:
			return errLocked
		}
	}

	var opts []etcd.OpOption
	// any way to avoid having to fetch the lock to know whether to
	// use a lease on the put?
	if ses := s.Get("/con/lock/" + req.path); ses != "" && ses[:1] == "d" {
		lid, err := leaseIDFromSession(ses)
		if err != nil {
			return err
		}
		opts = append(opts, etcd.WithLease(lid))
	}

	s.Put("/con/flags/"+req.path, encodeInt64(int64(req.Flags)), opts...)

	if req.datc != nil {
		v, ok := <-req.datc
		if !ok {
			return errBadBody
		}
		req.value, req.datc = v, nil
	}
	s.Put("/con/value/"+req.path, req.value, opts...)
	return nil
}

func (kvh *kvHandler) get(w http.ResponseWriter, req *getRequest) error {
	var opts []etcd.OpOption

	if len(req.Token) != 0 {
		panic("stub token")
	}
	if len(req.Dc) != 0 {
		panic("stub datacenter")
	}

	if !req.Consistent {
		// !Consistent && !Stale => default, a very broken mode.
		// Since it doesn't offer any real linearizability guarantees,
		// we're free to treat it as req.Stale
		opts = append(opts, etcd.WithSerializable())
	}

	if req.Keys {
		opts = append(opts, etcd.WithKeysOnly())
		req.Recurse = true
	}
	if req.Recurse {
		// X-Consul-Index corresponds to latest ModifyIndex within prefix
		opts = append(opts, etcd.WithPrefix())
		// consul overrides raw if recurse or keys is given
		req.Raw = false
	}

	if req.path == "" && !req.Recurse {
		return errMissingKeyName
	}

	// 0 => wait forever; the default, however, is 5 minutes
	wdur := time.Duration(5 * time.Minute)
	if len(req.Wait) != 0 {
		d, derr := time.ParseDuration(req.Wait)
		if derr != nil {
			return derr
		}
		wdur = d
	}

	if req.Index != 0 {
		// watch for first modify index > given index
		wctx, wcancel := context.WithCancel(req.ctx)
		defer wcancel()
		wchv := kvh.cli.Watch(wctx, "/con/value/"+req.path, etcd.WithRev(int64(req.Index+1)))
		wchl := kvh.cli.Watch(wctx, "/con/lock/"+req.path, etcd.WithRev(int64(req.Index+1)))
		tickerc := time.After(wdur)
		if wdur == 0 {
			tickerc = nil
		}
		select {
		case <-wchl:
		case <-wchv:
		case <-tickerc:
			return req.ctx.Err()
		}
	}

	resp, err := kvh.cli.Txn(req.ctx).Then(
		etcd.OpGet("/con/value/"+req.path, opts...),
		etcd.OpGet("/con/flags/"+req.path, opts...),
		etcd.OpGet("/con/lock/"+req.path, opts...),
		etcd.OpGet("/con/lidx/"+req.path, opts...),
	).Commit()
	if err != nil {
		// could be error that key does not exist
		return err
	}

	respValue := resp.Responses[0].GetResponseRange()
	respFlags := resp.Responses[1].GetResponseRange()
	respLock := resp.Responses[2].GetResponseRange()
	respLIdx := resp.Responses[3].GetResponseRange()

	if req.Raw {
		_, err = w.Write(respValue.Kvs[0].Value)
		return err
	}

	// respLock and respLIdx may not exist for every respValue; indexes may not match
	// instead, determine association by key lookup
	lockMap := make(map[string]string)
	for _, kv := range respLock.Kvs {
		k := strings.Replace(string(kv.Key), "/con/lock/", "", 1)
		lockMap[k] = string(kv.Value)
	}
	lidxMap := make(map[string]uint64)
	for _, kv := range respLIdx.Kvs {
		k := strings.Replace(string(kv.Key), "/con/lidx/", "", 1)
		lidxMap[k] = uint64(kv.Version)
	}

	resps := make([]getResponse, len(respValue.Kvs))
	for i := 0; i < len(respValue.Kvs); i++ {
		k := strings.Replace(string(respValue.Kvs[i].Key), "/con/value/", "", 1)
		resps[i] = getResponse{
			CreateIndex: uint64(respValue.Kvs[i].CreateRevision),
			ModifyIndex: uint64(respValue.Kvs[i].ModRevision),
			LockIndex:   lidxMap[k],
			Key:         k,
			Flags:       uint64(decodeInt64(respFlags.Kvs[i].Value)),
			Value:       respValue.Kvs[i].Value,
			Session:     lockMap[k],
		}
	}

	if req.Keys {
		var keys []string
		for _, resp := range resps {
			keys = append(keys, `"`+resp.Key+`"`)
		}
		if req.Separator != "" {
			var sepkeys []string
			sepset := make(map[string]struct{})
			for _, key := range keys {
				sp := strings.Split(key, req.Separator)
				if len(sp) == 1 {
					sepkeys = append(sepkeys, key)
					continue
				}
				k := sp[0] + req.Separator
				if _, ok := sepset[k]; ok {
					continue
				}
				sepkeys = append(sepkeys, k)
				sepset[k] = struct{}{}
			}
			keys = sepkeys
		}
		_, err = w.Write([]byte(`[` + strings.Join(keys, ",") + `]`))
		return err
	}

	var jsons []string
	for _, r := range resps {
		dat, merr := json.Marshal(r)
		if merr != nil {
			return merr
		}
		jsons = append(jsons, string(dat))
	}
	_, err = w.Write([]byte(`[` + strings.Join(jsons, ",") + `]`))
	return err
}

func (kvh *kvHandler) del(w http.ResponseWriter, req *delRequest) error {
	var opts []etcd.OpOption
	if req.Recurse {
		if _, ok := req.values["cas"]; ok {
			// HTTP 400
			return fmt.Errorf("Conflicting flags: cas=10&recurse")
		}
		opts = append(opts, etcd.WithPrefix())
	}
	txn := kvh.cli.Txn(req.ctx)
	if req.Cas != 0 {
		cmp := etcd.Compare(etcd.ModRevision("/con/values/"+req.path), "=", req.Cas)
		txn.If(cmp)
	}
	resp, err := txn.Then(
		etcd.OpDelete("/con/values/"+req.path, opts...),
		etcd.OpDelete("/con/flags/"+req.path, opts...),
		etcd.OpDelete("/con/lock/"+req.path, opts...),
		etcd.OpDelete("/con/lidx/"+req.path, opts...),
	).Commit()
	if err != nil {
		return err
	}
	if !resp.Succeeded {
		// cas failed
		_, err = w.Write([]byte("false"))
		return err
	}
	_, err = w.Write([]byte("true"))
	return err
}

// getRequest is sent via accessing /v1/key/<key>
type getRequest struct {
	path   string
	values url.Values
	ctx    context.Context

	// Consistent is true to do linearized reads.
	Consistent bool `schema:"consistent"`
	// Stale is for serialized reads.
	Stale bool `schema:"stale"`
	// Token is the acl token.
	Token string `schema:"token"`
	// Dc is the datacenter.
	Dc string `schema:"dc"`
	// Recurse returns all keys with given prefix.
	Recurse bool `schema:"recurse"`
	// Raw is the value of the key without any encoding.
	Raw bool `schema:"raw"`
	// Keys lists just keys without values.
	Keys bool `schema:"keys"`
	// Separator lists only up to a given separator.
	// Requesting '/a/' with separator '/' => [ "/a/1","/a/2", "/a/dir/"].
	Separator string `schema:"separator"`
	// Wait defines how long to wait for an update (e.g., wait=5s).
	Wait string `schema:"wait"`
	// Index is the modified index for waiting.
	Index uint64 `schema:"index"`
}

type getResponse struct {
	CreateIndex uint64
	ModifyIndex uint64
	LockIndex   uint64
	Key         string
	Flags       uint64
	Value       []byte // base64
	Session     string // "adf4238a-882b-9ddc-4a9d-5b6758e4159e"
}

type putRequest struct {
	path   string
	value  string
	datc   chan string
	values url.Values
	ctx    context.Context

	// Flags specifies an unsigned value between 0 and (2^64)-1.
	Flags uint64 `schema:"flags"`

	// Cas = <index> : turn the PUT into a Check-And-Set.
	Cas uint64 `schema:"cas"`

	// Release turns the PUT into a lock release. LockIndex stays unmodified but
	// the key's Session is cleared.
	Release string `schema:"release"`

	// Acquire=<session> is used to turn the PUT into a lock acquisition.
	Acquire string `schema:"acquire"`
}

// delRequest deletes a single key or all keys sharing a prefix.
type delRequest struct {
	path   string
	values url.Values
	ctx    context.Context

	// Recurse deletes all keys with the path as a prefix.
	Recurse bool `schema:"recurse"`

	// Cas <index> turns the delete into a Check-And-Set. Index must be >0 to
	// take action; 0 is a nop. Key deleted iff given index matches ModifyIndex.
	Cas uint64 `schema:"cas"`
}

func decodeInt64(v []byte) int64 { x, _ := binary.Varint(v); return x }

func encodeInt64(v int64) string {
	b := make([]byte, binary.MaxVarintLen64)
	return string(b[:binary.PutVarint(b, v)])
}
