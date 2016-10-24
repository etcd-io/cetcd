#!/bin/bash
curl -v http://127.0.0.1:8500/v1/kv/?recurse 
curl -X PUT -d 'test' http://127.0.0.1:8500/v1/kv/web/key1 >/dev/null 2>&1
curl -X PUT -d 'test' http://127.0.0.1:8500/v1/kv/web/key2?flags=42
curl -X PUT -d 'test'  http://127.0.0.1:8500/v1/kv/web/sub/key3
curl http://127.0.0.1:8500/v1/kv/?recurse