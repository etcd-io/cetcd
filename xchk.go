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
	"bytes"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httputil"
	"net/url"
	"reflect"
	"sync"
)

type Xchk struct {
	oracleURL url.URL
	mu        sync.Mutex
}

func NewXchk(oracle url.URL) *Xchk { return &Xchk{oracleURL: oracle} }

type xchkHandler struct {
	xchk      *Xchk
	oracle    http.Handler
	candidate http.Handler
}

func (xchk *Xchk) Handler(candidate http.Handler) http.Handler {
	rp := httputil.NewSingleHostReverseProxy(&xchk.oracleURL)
	return &xchkHandler{xchk: xchk, oracle: rp, candidate: candidate}
}

func (xh *xchkHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var wg sync.WaitGroup
	xh.xchk.mu.Lock()
	defer xh.xchk.mu.Unlock()
	rbuf, err := ioutil.ReadAll(r.Body)
	if err != nil {
		panic(err)
	}
	rdr1 := ioutil.NopCloser(bytes.NewBuffer(rbuf))
	rdr2 := ioutil.NopCloser(bytes.NewBuffer(rbuf))
	wg.Add(2)

	// TODO if request has wait, need to drop cluster-wide lock
	// TODO if has session need correspondence map

	ow, cw := newXchkResponseWriter(), newXchkResponseWriter()
	go func() {
		defer wg.Done()
		ro := *r
		ro.Body = rdr1
		xh.oracle.ServeHTTP(ow, &ro)
	}()
	go func() {
		defer wg.Done()
		rc := *r
		rc.Body = rdr2
		xh.candidate.ServeHTTP(cw, &rc)
	}()
	wg.Wait()

	ostr, cstr := ow.buf.String(), cw.buf.String()
	if !reflect.DeepEqual(ostr, cstr) {
		fmt.Printf("mismatch expected %q, got %q\n", ostr, cstr)
	}
}

type teeResponseWriter struct{ rw []http.ResponseWriter }

func newTeeResponseWriter(rws []http.ResponseWriter) http.ResponseWriter {
	return &teeResponseWriter{rw: rws}
}
func (trw *teeResponseWriter) Header() http.Header { return trw.rw[0].Header() }

func (trw *teeResponseWriter) WriteHeader(n int) {
	for _, rw := range trw.rw {
		for k, v := range trw.rw[0].Header() {
			rw.Header()[k] = v
		}
		rw.WriteHeader(n)
	}
}

func (trw *teeResponseWriter) Write(b []byte) (int, error) {
	for _, rw := range trw.rw[1:] {
		rw.Write(b)
	}
	return trw.rw[0].Write(b)
}

type xchkResponseWriter struct {
	hdr    http.Header
	status int
	buf    bytes.Buffer
}

func newXchkResponseWriter() *xchkResponseWriter {
	return &xchkResponseWriter{hdr: make(http.Header)}
}
func (xrw *xchkResponseWriter) Header() http.Header         { return xrw.hdr }
func (xrw *xchkResponseWriter) WriteHeader(n int)           { xrw.status = n }
func (xrw *xchkResponseWriter) Write(b []byte) (int, error) { return xrw.buf.Write(b) }
