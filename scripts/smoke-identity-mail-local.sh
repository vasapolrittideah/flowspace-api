#!/bin/sh
set -eu
umask 077

context=k3d-flowspace
namespace=flowspace-local
api_port=18082
mail_port=18025
response=$(mktemp)

cleanup() {
  kubectl --context "$context" -n "$namespace" scale statefulset/redpanda --replicas=1 >/dev/null 2>&1 || true
  kubectl --context "$context" -n "$namespace" scale deployment/mailpit --replicas=1 >/dev/null 2>&1 || true
  kill "${api_forward:-}" "${mail_forward:-}" 2>/dev/null || true
  rm -f "$response"
}
trap cleanup EXIT HUP INT TERM

kubectl --context "$context" -n "$namespace" wait --for=condition=complete job/identity-broker-bootstrap --timeout=180s
kubectl --context "$context" -n "$namespace" rollout status deployment/identity-api --timeout=180s
kubectl --context "$context" -n "$namespace" rollout status deployment/identity-worker --timeout=180s
kubectl --context "$context" -n "$namespace" rollout status deployment/mailpit --timeout=180s

kubectl --context "$context" -n "$namespace" port-forward --address 127.0.0.1 svc/identity-api "$api_port:8080" >/dev/null 2>&1 &
api_forward=$!
kubectl --context "$context" -n "$namespace" port-forward --address 127.0.0.1 svc/mailpit-http "$mail_port:8025" >/dev/null 2>&1 &
mail_forward=$!

mail_count() {
  curl --fail --silent --show-error --max-time 5 "http://127.0.0.1:$mail_port/api/v1/messages" | jq -e '.total'
}

wait_for_mail() {
  expected=$1
  for attempt in $(seq 1 180); do
    if count=$(mail_count 2>/dev/null) && [ "$count" -ge "$expected" ]; then
      return 0
    fi
    sleep 1
  done
  printf 'Mailpit did not receive the queued email.\n' >&2
  return 1
}

signup() {
  email="smoke-$(openssl rand -hex 8)@example.test"
  password=$(openssl rand -hex 20)
  status=$(jq -nc --arg email "$email" --arg password "$password" '{email:$email,password:$password}' |
    curl --silent --show-error --max-time 15 --output "$response" --write-out '%{http_code}' \
      --header 'Content-Type: application/json' --data-binary @- "http://127.0.0.1:$api_port/v1/accounts")
  if [ "$status" != 200 ] || ! jq -e '.subject != "" and .emailVerified == false and .accessToken != "" and .refreshToken != ""' "$response" >/dev/null; then
    printf 'Signup failed during outage (HTTP %s).\n' "$status" >&2
    return 1
  fi
}

wait_for_mail 0
before=$(mail_count)
kubectl --context "$context" -n "$namespace" scale statefulset/redpanda --replicas=0
kubectl --context "$context" -n "$namespace" wait --for=delete pod/redpanda-0 --timeout=120s
signup
test "$(mail_count)" -eq "$before"
kubectl --context "$context" -n "$namespace" scale statefulset/redpanda --replicas=1
kubectl --context "$context" -n "$namespace" rollout status statefulset/redpanda --timeout=180s
wait_for_mail "$((before + 1))"
printf 'Broker outage: signup committed and email arrived after recovery.\n'

kubectl --context "$context" -n "$namespace" scale deployment/mailpit --replicas=0
kubectl --context "$context" -n "$namespace" wait --for=delete pod -l app.kubernetes.io/name=mailpit --timeout=120s
signup
kubectl --context "$context" -n "$namespace" scale deployment/mailpit --replicas=1
kubectl --context "$context" -n "$namespace" rollout status deployment/mailpit --timeout=180s
kubectl --context "$context" -n "$namespace" port-forward --address 127.0.0.1 svc/mailpit-http "$mail_port:8025" >/dev/null 2>&1 &
mail_forward=$!
wait_for_mail 1
printf 'Mailpit outage: signup committed and email arrived after recovery.\n'
