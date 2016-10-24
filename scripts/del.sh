#!/bin/bash
for a in `seq 0 100`; do
	curl -X PUT -d 'test' http://127.0.0.1:8500/v1/kv/web/key$a >/dev/null 2>&1
done
curl -X DELETE "http://127.0.0.1:8500/v1/kv/web/key?recurse&cas=10"
curl http://127.0.0.1:8500/v1/kv/?recurse
echo
date