#!/bin/bash
curl -X PUT -d 'test' http://127.0.0.1:8500/v1/kv/web/key1 >/dev/null 2>&1
curl -X PUT -d 'test' http://127.0.0.1:8500/v1/kv/web/key1 >/dev/null 2>&1
modidx=`curl http://127.0.0.1:8500/v1/kv/web/key1 2>/dev/null | sed 's/,/\r\n/g' | grep ModifyIndex | cut -f2 -d':' | sed 's/[^0-9]//g'`

echo wait 1s on no new events
curl "http://127.0.0.1:8500/v1/kv/web/key1?index=$modidx&wait=1s"; echo

echo wait index=modidx - 2
oldmod=`expr $modidx - 2`
curl "http://127.0.0.1:8500/v1/kv/web/key1?index=$oldmod&wait=100s"; echo


echo wait index=0
curl "http://127.0.0.1:8500/v1/kv/web/key1?index=0&wait=100s"; echo

echo wait forever at `date` on $modidx
(curl "http://127.0.0.1:8500/v1/kv/web/key1?index=$modidx"; echo ) &
wpid=$!
sleep 2s
curl -X PUT -d 'test2' http://127.0.0.1:8500/v1/kv/web/key1 >/dev/null 2>&1; echo
newmodidx=`curl http://127.0.0.1:8500/v1/kv/web/key1 2>/dev/null | sed 's/,/\r\n/g' | grep ModifyIndex | cut -f2 -d':' | sed 's/[^0-9]//g'`
wait $wpid
echo done waiting at `date`". got new modidx=$newmodidx"