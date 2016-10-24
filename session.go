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
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"
	"time"

	etcd "github.com/coreos/etcd/clientv3"
	"github.com/coreos/etcd/etcdserver/api/v3rpc/rpctypes"
	"github.com/coreos/etcd/mvcc/mvccpb"
	"golang.org/x/net/context"
)

func NewSessionHandler(cli *etcd.Client) http.Handler {
	return &sessionHandler{cli}
}

type sessionHandler struct {
	cli *etcd.Client
}

func (sh *sessionHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var err error
	path := strings.Replace(r.URL.Path, "/v1/session/", "", 1)
	parts := strings.Split(path, "/")
	switch r.Method {
	case "PUT":
		switch parts[0] {
		case "create":
			err = sh.create(w, r)
		case "destroy":
			err = sh.destroy(w, parts[1])
		case "renew":
			err = sh.renew(w, parts[1])
		}
	case "GET":
		switch parts[0] {
		case "info":
			err = sh.info(w, parts[1])
		case "node":
			err = sh.node(w, parts[1])
		case "list":
			err = sh.list(w)
		}
	default:
		err = fmt.Errorf("unknown method %q", r.Method)
	}
	if err != nil {
		http.Error(w, err.Error(), 400)
	}
}

func (sh *sessionHandler) create(w http.ResponseWriter, r *http.Request) error {
	dat, err := ioutil.ReadAll(r.Body)
	if err != nil {
		return err
	}
	req := createRequest{}
	if err = json.Unmarshal(dat, &req); err != nil {
		return err
	}
	if len(req.LockDelay) == 0 {
		// not specified, use default
		req.LockDelay = "15s"
	}
	if len(req.Node) != 0 {
		panic("stub")
	}
	if len(req.Checks) == 0 {
		req.Checks = []string{"serfHealth"}
	}
	switch req.Behavior {
	case "release":
	case "delete":
	case "":
		req.Behavior = "release"
	default:
		return fmt.Errorf("Invalid Behavior setting '%s'", req.Behavior)
	}

	if len(req.TTL) == 0 {
		req.TTL = "10s"
	}
	dur, err := time.ParseDuration(req.TTL)
	if err != nil {
		return err
	}
	ttl := int64(dur.Seconds())
	if ttl < 10 || ttl > 86400 {
		// must be between 10s and 86400s when provided
		return fmt.Errorf("Invalid Session TTL '%d', must be between [10s=24h0m0s]", dur)
	}

	lresp, lerr := sh.cli.Grant(r.Context(), ttl)
	if lerr != nil {
		return lerr
	}

	id := fmt.Sprintf("%s-%x", req.Behavior[:1], lresp.ID)
	jenc, merr := json.Marshal(req)
	if merr != nil {
		return merr
	}
	_, perr := sh.cli.Put(r.Context(), "/con/sess/"+id, string(jenc), etcd.WithLease(lresp.ID))
	if perr != nil {
		return perr
	}

	_, werr := w.Write([]byte(`{"ID":"` + id + `"}`))
	return werr
}

func (sh *sessionHandler) destroy(w http.ResponseWriter, ses string) error {
	// delete the /con/sess and revoke its lease
	id, err := leaseIDFromSession(ses)
	if err != nil {
		return err
	}
	_, err = sh.cli.Revoke(context.TODO(), id)
	if err != nil && err != rpctypes.ErrLeaseNotFound {
		return err
	}
	_, err = w.Write([]byte("true"))
	return err
}

func (sh *sessionHandler) renew(w http.ResponseWriter, ses string) error {
	// renew is a straight shot to etcd lease handling machinery
	id, err := leaseIDFromSession(ses)
	if err != nil {
		return err
	}
	if _, err = sh.cli.KeepAliveOnce(context.TODO(), id); err != nil {
		return err
	}
	return sh.info(w, ses)
}

// info returns information for a given session.
func (sh *sessionHandler) info(w http.ResponseWriter, ses string) error {
	resp, err := sh.cli.Get(context.TODO(), "/con/sess/"+ses)
	if err != nil {
		return err
	}
	inf, ierr := sessionKVToInfo(resp.Kvs[0])
	if ierr != nil {
		return ierr
	}
	m, merr := json.Marshal(*inf)
	if merr != nil {
		return merr
	}
	_, err = w.Write(m)
	return err
}

// node returns the active sessions for a given node and datacenter.
// Stub for now.
func (sh *sessionHandler) node(w http.ResponseWriter, node string) error {
	return sh.list(w)
}

// list returns the active sessions for a given datacenter.
func (sh *sessionHandler) list(w http.ResponseWriter) error {
	resp, err := sh.cli.Get(context.TODO(), "/con/sess/", etcd.WithPrefix())
	if err != nil {
		return err
	}
	var infos []infoResponse
	for _, kv := range resp.Kvs {
		inf, ierr := sessionKVToInfo(kv)
		if ierr != nil {
			fmt.Println("session error:", ierr)
			continue
			//	return ierr
		}
		infos = append(infos, *inf)
	}
	m, merr := json.Marshal(infos)
	if merr != nil {
		return merr
	}
	_, err = w.Write(m)
	return err
}

func sessionKVToInfo(kv *mvccpb.KeyValue) (*infoResponse, error) {
	cr := createRequest{}
	if err := json.Unmarshal(kv.Value, &cr); err != nil {
		return nil, err
	}
	if len(cr.LockDelay) == 0 {
		cr.LockDelay = "0"
	}
	dur, derr := time.ParseDuration(cr.LockDelay)
	if derr != nil {
		return nil, derr
	}
	return &infoResponse{
		ID:          strings.Replace(string(kv.Key), "/con/sess/", "", 1),
		LockDelay:   float64(dur),
		Checks:      cr.Checks,
		Node:        cr.Node,
		CreateIndex: uint64(kv.CreateRevision),
		ModifyIndex: uint64(kv.CreateRevision),
	}, nil
}

// createRequest initializes a new session.
type createRequest struct {
	// LockDelay can be specified as a duration string using a "s" suffix for seconds. The default is 15s.
	// It is between 0 and 60 seconds; on session invalidation, previously held locks are held until the lock-delay interval.
	LockDelay string
	// Name can be used to provide a human-readable name for the Session.
	Name string
	// Node refers to a registered node. By default, uses the agent's own node.
	Node string
	// Checks provides a list of health checks. Defaults to serfHealth.
	Checks []string
	// Behavior controls session invalidation. "delete" will delete all held locks.
	// "release" will release held locks. Defaults to  "release".
	Behavior string
	// TTL is the duration of the lease.
	TTL string
	// Uses agent's local datacenter by default; another can be given with "?dc=".
}

type createResponse struct {
	// ID of the created session.
	ID string
}

type infoResponse struct {
	LockDelay   float64 // nanoseconds
	Checks      []string
	Node        string
	ID          string
	CreateIndex uint64
	ModifyIndex uint64
}

func leaseIDFromSession(s string) (etcd.LeaseID, error) {
	id := etcd.LeaseID(0)
	sp := strings.Split(s, "-")
	if len(sp) != 2 {
		return id, fmt.Errorf("malformed session")
	}
	if _, err := fmt.Sscanf(sp[1], "%x", &id); err != nil {
		return id, err
	}
	return id, nil
}
