#!/bin/sh
set -eu

context=k3d-flowspace
namespace=flowspace-local
port=18082

kubectl --context "$context" -n "$namespace" wait --for=condition=complete job/identity-migrate --timeout=180s
kubectl --context "$context" -n "$namespace" rollout status deployment/identity-api --timeout=180s

kubectl --context "$context" -n "$namespace" port-forward --address 127.0.0.1 svc/identity-api "$port:8080" >/dev/null 2>&1 &
forward=$!
trap 'kill "$forward" 2>/dev/null || true' EXIT HUP INT TERM

ready=0
for attempt in 1 2 3 4 5 6 7 8 9 10; do
  if curl --silent --output /dev/null "http://127.0.0.1:$port/v1/accounts"; then
    ready=1
    break
  fi
  sleep 1
done
test "$ready" -eq 1

route=$(curl --silent --output /dev/null --write-out '%{http_code}' \
  --header 'Content-Type: application/json' --data '{}' "http://127.0.0.1:$port/v1/accounts")
test "$route" = 400

internal=$(curl --silent --output /dev/null --write-out '%{http_code}' "http://127.0.0.1:$port/readyz")
test "$internal" = 404
jwks=$(curl --silent --output /dev/null --write-out '%{http_code}' "http://127.0.0.1:$port/.well-known/jwks.json")
test "$jwks" = 404

printf 'Identity migration, API readiness, public route, and internal-route isolation passed.\n'
