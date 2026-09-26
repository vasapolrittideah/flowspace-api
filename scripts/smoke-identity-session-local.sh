#!/bin/sh
set -eu

context=k3d-flowspace
namespace=flowspace-local
certs=.secrets/identity-session
test "$#" -eq 0 || test "$#" -eq 2 || test "$#" -eq 5
workspace_cert=${1:-$certs/workspace.crt}
workspace_key=${2:-$certs/workspace.key}
ca_cert=${3:-$certs/ca.crt}
unapproved_cert=${4:-$certs/unapproved.crt}
unapproved_key=${5:-$certs/unapproved.key}
descriptor=$(mktemp)
trap 'rm -f "$descriptor"; kill "${forward:-}" 2>/dev/null || true' EXIT HUP INT TERM

bin/buf build --as-file-descriptor-set --output "$descriptor"
kubectl --context "$context" -n "$namespace" rollout status deployment/identity-api --timeout=180s
kubectl --context "$context" -n "$namespace" get service identity-session >/dev/null
kubectl --context "$context" -n "$namespace" port-forward --address 127.0.0.1 svc/identity-session 18083:8082 >/dev/null 2>&1 &
forward=$!

for attempt in 1 2 3 4 5 6 7 8 9 10; do
  if nc -z 127.0.0.1 18083 2>/dev/null; then
    break
  fi
  sleep 1
done
nc -z 127.0.0.1 18083

request='{"subject":"missing-subject","sessionId":"00000000-0000-0000-0000-000000000000"}'
approved=$(grpcurl -max-time 5 -authority identity-session.flowspace-local.svc \
  -cacert "$ca_cert" -cert "$workspace_cert" -key "$workspace_key" \
  -protoset "$descriptor" -d "$request" 127.0.0.1:18083 \
  flowspace.identity.v1.IdentityService/CheckSession 2>&1) && exit 1
printf '%s' "$approved" | grep -q 'Code: Unauthenticated'

unapproved=$(grpcurl -max-time 5 -authority identity-session.flowspace-local.svc \
  -cacert "$ca_cert" -cert "$unapproved_cert" -key "$unapproved_key" \
  -protoset "$descriptor" -d "$request" 127.0.0.1:18083 \
  flowspace.identity.v1.IdentityService/CheckSession 2>&1) && exit 1
if printf '%s' "$unapproved" | grep -q 'Code: Unauthenticated'; then
  exit 1
fi

printf 'Approved client reached CheckSession; unapproved client failed TLS authentication.\n'
