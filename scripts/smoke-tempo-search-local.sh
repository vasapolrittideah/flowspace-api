#!/bin/sh
set -eu

# The script sends Identity requests to create traces, then runs parallel
# Tempo searches. It fails when Tempo restarts or does not answer a search
# after the parallel searches. Some parallel searches can fail while Tempo is
# busy, so the script does not count them.
context=k3d-flowspace
namespace=flowspace-local
requests=${TEMPO_SMOKE_REQUESTS:-3000}
trap 'kill "${identity_forward:-}" "${tempo_forward:-}" 2>/dev/null || true' EXIT HUP INT TERM

kubectl --context "$context" -n "$namespace" rollout status statefulset/tempo --timeout=180s
kubectl --context "$context" -n "$namespace" rollout status deployment/identity-api --timeout=180s
restarts() {
  kubectl --context "$context" -n "$namespace" get pod tempo-0 -o jsonpath='{.status.containerStatuses[0].restartCount}'
}
before=$(restarts)

kubectl --context "$context" -n "$namespace" port-forward --address 127.0.0.1 svc/identity-api 18084:8080 >/dev/null 2>&1 &
identity_forward=$!
kubectl --context "$context" -n "$namespace" port-forward --address 127.0.0.1 svc/tempo 13200:3200 >/dev/null 2>&1 &
tempo_forward=$!
for port in 18084 13200; do
  for attempt in 1 2 3 4 5 6 7 8 9 10; do
    if nc -z 127.0.0.1 "$port" 2>/dev/null; then
      break
    fi
    sleep 1
  done
  nc -z 127.0.0.1 "$port"
done

seq 1 "$requests" | xargs -P 20 -I{} curl -s -o /dev/null -H 'Content-Type: application/json' \
  -d '{"email":"x"}' http://127.0.0.1:18084/v1/password-reset-codes

# Empty searches also succeed, so wait until a new trace is in Tempo.
trace_id=$(od -An -N16 -tx1 /dev/urandom | tr -d ' \n')
curl -s -o /dev/null -H "traceparent: 00-$trace_id-00f067aa0ba902b7-01" \
  -H 'Content-Type: application/json' -d '{"email":"x"}' http://127.0.0.1:18084/v1/password-reset-codes
for attempt in $(seq 1 30); do
  if [ "$(curl -s -o /dev/null -w '%{http_code}' --max-time 10 "http://127.0.0.1:13200/api/traces/$trace_id")" = 200 ]; then
    break
  fi
  if [ "$attempt" -eq 30 ]; then
    printf 'Tempo does not have the new trace %s after 30 attempts.\n' "$trace_id" >&2
    exit 1
  fi
  sleep 2
done

end=$(date +%s)
start=$((end - 10800))
for round in 1 2 3; do
  set --
  for search in $(seq 1 15); do
    curl -s -o /dev/null --max-time 60 \
      "http://127.0.0.1:13200/api/search?limit=5000&start=$start&end=$end" &
    set -- "$@" "$!"
    curl -s -o /dev/null --max-time 60 \
      "http://127.0.0.1:13200/api/search?q=%7B%7D&limit=5000&spss=100&start=$start&end=$end" &
    set -- "$@" "$!"
  done
  wait "$@" || true
  sleep 10
done
sleep 5

answer=$(curl -s -o /dev/null -w '%{http_code}' --max-time 30 \
  "http://127.0.0.1:13200/api/search?limit=1&start=$start&end=$end" || true)
after=$(restarts)
if [ "$answer" != 200 ] || [ "$after" != "$before" ]; then
  printf 'Tempo restarted %s times, and its last search returned %s.\n' "$((after - before))" "$answer" >&2
  exit 1
fi
printf 'Tempo answered after 90 parallel searches without a restart.\n'
