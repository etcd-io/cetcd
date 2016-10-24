#!/bin/bash

id=`curl -vs  -X PUT -d '{"TTL":"10s","LockDelay":"0s"}' http://127.0.0.1:8500/v1/session/create 2>/dev/null | tail -n1 | cut -f2 -d: | cut -f1 -d'}' | sed 's/"//g'`
id=$id
id2=`curl -vs  -X PUT -d '{"TTL":"15s","LockDelay":"0s"}' http://127.0.0.1:8500/v1/session/create 2>/dev/null | tail -n1 | cut -f2 -d: | cut -f1 -d'}' | sed 's/"//g'`
id2=$id2

echo before acquire
curl http://127.0.0.1:8500/v1/kv/web/key1 2>/dev/null; echo
echo acquiring $id
curl -X PUT -d 'abcd' "http://127.0.0.1:8500/v1/kv/web/key1?acquire=$id" >/dev/null 2>&1
curl http://127.0.0.1:8500/v1/kv/web/key1 2>/dev/null; echo
echo releasing $id
curl -X PUT "http://127.0.0.1:8500/v1/kv/web/key1?release=$id" >/dev/null 2>&1
curl http://127.0.0.1:8500/v1/kv/web/key1 2>/dev/null; echo

echo acquiring again, $id
curl -X PUT -d 'dddd' "http://127.0.0.1:8500/v1/kv/web/key1?acquire=$id" >/dev/null 2>&1; echo
curl http://127.0.0.1:8500/v1/kv/web/key1 2>/dev/null; echo
echo
echo try to acquire with $id2 SHOULD FAIL
curl -X PUT -d 'abcd' "http://127.0.0.1:8500/v1/kv/web/key1?acquire=$id2" 2>/dev/null; echo
curl http://127.0.0.1:8500/v1/kv/web/key1 2>/dev/null; echo
echo
echo try to write without session
curl -X PUT -d 'mmmm' "http://127.0.0.1:8500/v1/kv/web/key1" 2>/dev/null; echo
curl http://127.0.0.1:8500/v1/kv/web/key1 2>/dev/null; echo