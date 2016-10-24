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
	"fmt"
	"net/http"

	etcd "github.com/coreos/etcd/clientv3"
)

type txnHandler struct{ cli *etcd.Client }

func NewTxnHandler(cli *etcd.Client) http.Handler { return &txnHandler{cli} }

func (th *txnHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	fmt.Println(r.Method)
	panic("stub")
	switch r.Method {
	case "PUT":
	case "GET":
	case "DELETE":
	default:
	}
}
