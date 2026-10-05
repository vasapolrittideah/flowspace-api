#!/bin/sh
set -eu

# The script stops Tempo and sends large spans to Alloy until the memory
# limiter of Alloy refuses them. It fails when Alloy never refuses spans, or
# when the Alloy pod is not ready, restarts, or is replaced. It starts Tempo
# again on exit.
context=k3d-flowspace
namespace=flowspace-local
for tool in kubectl nc go; do
  command -v "$tool" >/dev/null || { printf 'The script needs %s.\n' "$tool" >&2; exit 1; }
done

cleanup() {
  for process in ${alloy_forward:-} ${watch:-}; do
    kill "$process" 2>/dev/null || true
    wait "$process" 2>/dev/null || true
  done
  rm -f "${failures:-}"
  kubectl --context "$context" -n "$namespace" scale statefulset/tempo --replicas=1 >/dev/null
}
trap cleanup EXIT
trap 'exit 1' HUP INT TERM

kubectl --context "$context" -n "$namespace" rollout status deployment/alloy --timeout=180s
pod=$(kubectl --context "$context" -n "$namespace" get pod -l app.kubernetes.io/name=alloy -o jsonpath='{.items[0].metadata.name}')
# Each sample is the pod UID, the readiness, and the restart count of the alloy container.
sample() {
  kubectl --context "$context" -n "$namespace" get pod "$pod" \
    -o jsonpath='{.metadata.uid} {.status.containerStatuses[?(@.name=="alloy")].ready} {.status.containerStatuses[?(@.name=="alloy")].restartCount}'
}
expected=$(sample)
case "$expected" in
  *" true "*) ;;
  *) printf 'Alloy pod %s is not ready: %s\n' "$pod" "$expected" >&2; exit 1 ;;
esac

kubectl --context "$context" -n "$namespace" scale statefulset/tempo --replicas=0
kubectl --context "$context" -n "$namespace" wait --for=delete pod/tempo-0 --timeout=120s
kubectl --context "$context" -n "$namespace" port-forward --address 127.0.0.1 "pod/$pod" 14317:4317 >/dev/null 2>&1 &
alloy_forward=$!
for attempt in 1 2 3 4 5 6 7 8 9 10; do
  if nc -z 127.0.0.1 14317 2>/dev/null; then
    break
  fi
  sleep 1
done
nc -z 127.0.0.1 14317

# A failed query counts as a failed sample.
failures=$(mktemp)
(
  while true; do
    current=$(sample 2>/dev/null) || current="query failed"
    [ "$current" = "$expected" ] || printf '%s\n' "$current" >>"$failures"
    sleep 2
  done
) &
watch=$!

ALLOY_OTLP_ENDPOINT=127.0.0.1:14317 go test -tags=smoke -count=1 -timeout=5m ./tests/smoke/alloy/
sleep 20
current=$(sample 2>/dev/null) || current="query failed"
[ "$current" = "$expected" ] || printf '%s\n' "$current" >>"$failures"

if [ -s "$failures" ]; then
  printf 'Alloy pod %s left its ready state in %s samples. First sample: %s\n' \
    "$pod" "$(wc -l <"$failures" | tr -d ' ')" "$(head -n 1 "$failures")" >&2
  exit 1
fi
printf 'Alloy refused spans at its memory limit, stayed ready, and did not restart.\n'
