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

package main

import (
	"flag"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"

	"github.com/coreos/cetcd"

	etcd "github.com/coreos/etcd/clientv3"
)

func main() {
	// setup command line flags
	etcdAddrFl := flag.String("etcd", "http://localhost:2379", "etcd3 client endpoint")
	consulAddrFl := flag.String("consuladdr", "localhost:8500", "address for serving consul clients")
	xchkFl := flag.String("oracle", "", "oracle console endpoint for cross checking")
	flag.Parse()

	etcdAddr := *etcdAddrFl
	consulAddr := *consulAddrFl
	oracleAddr := *xchkFl

	// set up server
	cli, err := etcd.New(etcd.Config{Endpoints: []string{etcdAddr}})
	if err != nil {
		fmt.Println("oops", err)
		os.Exit(1)
	}

	ln, err := net.Listen("tcp", consulAddr)
	if err != nil {
		os.Exit(1)
	}

	kv := cetcd.NewKVHandler(cli)
	ses := cetcd.NewSessionHandler(cli)
	txn := cetcd.NewTxnHandler(cli)
	if len(oracleAddr) != 0 {
		url, uerr := url.Parse(oracleAddr)
		if uerr != nil {
			fmt.Println(err)
			os.Exit(1)
		}
		xchk := cetcd.NewXchk(*url)
		kv = xchk.Handler(kv)
		ses = xchk.Handler(ses)
		txn = xchk.Handler(txn)
	}

	mux := http.NewServeMux()
	mux.Handle("/v1/kv/", kv)
	mux.Handle("/v1/session/", ses)
	mux.Handle("/v1/txn/", txn)
	s := &http.Server{Handler: mux}
	err = s.Serve(ln)
	fmt.Println(err)
}
