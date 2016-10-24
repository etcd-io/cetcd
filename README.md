# cetcd

A consul "personality" for etcd. Point a consul client at cetcd to dispatch the operations on an etcd cluster.

## Usage

Forwarding consul requests on `:8500` to an etcd server listening on `localhost:2379`:

```sh
go install github.com/coreos/cetcd/cmd/cetcd
cetcd -etcd localhost:2379  -consuladdr 0.0.0.0:8500
```

Cross-checking consul emulation with a native consul server on `127.0.0.1:8501`:

```sh
cetcd -etcd localhost:2379  -consuladdr 0.0.0.0:8500 -oracle 127.0.0.1:8501
```

Simple testing with `curl`:

```sh
goreman start
curl -X PUT -d 'test' http://127.0.0.1:8500/v1/kv/testkey?flags=42
curl http://127.0.0.1:8500/v1/kv/testkey
```
