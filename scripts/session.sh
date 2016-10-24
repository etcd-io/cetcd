#!/bin/bash

curl -vs  -X PUT -d '{"TTL" : "10s", "Name": "bob", "Behavior" : "delete"}' http://127.0.0.1:8500/v1/session/create; echo
curl -vs  -X PUT -d '{"TTL" : "10s", "Name": "bob", "Behavior" : "release"}' http://127.0.0.1:8500/v1/session/create; echo
# 'Checks': source data must be an array or slice, got string
curl -vs  -X PUT -d '{"Checks" : "abc"}' http://127.0.0.1:8500/v1/session/create; echo
# Missing check 'abc' registration
curl -vs  -X PUT -d '{"Checks" : ["abc"]}' http://127.0.0.1:8500/v1/session/create; echo
# Invalid behavior setting
curl -vs  -X PUT -d '{"Behavior" : "qqq"}' http://127.0.0.1:8500/v1/session/create; echo
curl -vs  -X PUT -d '{"TTL" : "10ms"}' http://127.0.0.1:8500/v1/session/create; echo
curl -vs -X PUT -d '{"TTL" : "x"}' http://127.0.0.1:8500/v1/session/create; echo

# test list
curl -vs  http://127.0.0.1:8500/v1/session/list; echo
{"LockDelay":1.5e+10,"Checks":["serfHealth"],"Node":"","ID":"r-10f757e03e3bb412","CreateIndex":5}

[{"ID":"4c1f7d7f-8d4c-bf1c-c2fa-e4f234579770","Name":"","Node":"0d6fd3584206","Checks":["serfHealth"],"LockDelay":15000000000,"Behavior":"release","TTL":"","CreateIndex":8,"ModifyIndex":8}]


# test renew
id=`curl -vs  -X PUT -d '{}' http://127.0.0.1:8500/v1/session/create | tail -n1 | cut -f2 -d: | cut -f1 -d'}' | sed 's/"//g'`
id=$id
curl -vs -X PUT  http://127.0.0.1:8500/v1/session/renew/$id | tail -n1; echo
curl -vs -X PUT -d '{"Name":"abc"}'  http://127.0.0.1:8500/v1/session/renew/$id | tail -n1; echo
curl -vs -X PUT  http://127.0.0.1:8500/v1/session/renew/$id | tail -n1; echo

# test info
curl -vs -X PUT  http://127.0.0.1:8500/v1/session/renew/$id | tail -n1; echo

# test destroy
curl -vs -X PUT  http://127.0.0.1:8500/v1/session/destroy/$id | tail -n1; echo
curl -vs -X PUT  http://127.0.0.1:8500/v1/session/destroy/$id | tail -n1; echo
