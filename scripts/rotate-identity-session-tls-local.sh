#!/bin/sh
set -eu

cd "$(dirname "$0")/.."
umask 077

if test "$#" -ne 1; then
  printf 'Usage: %s identity|workspace|workspace-revoke\n' "$0" >&2
  exit 2
fi
revoke_first=0
case "$1" in
  identity) name=identity; cn=identity-session; purpose=sslserver ;;
  workspace) name=workspace; cn=workspace-api; purpose=sslclient ;;
  workspace-revoke) name=workspace; cn=workspace-api; purpose=sslclient; revoke_first=1 ;;
  *) printf 'Unknown certificate: %s\n' "$1" >&2; exit 2 ;;
esac

context=k3d-flowspace
namespace=flowspace-local
certs=.secrets/identity-session
config=deploy/overlays/local/identity/kustomization.yaml
manifest="deploy/overlays/local/$name/session-tls-sealed-secret.yaml"
secret="$name-session-tls"
fail() { printf '%s\n' "$1" >&2; exit 1; }
test "$(kubectl config current-context)" = "$context"
for file in ca.crt ca.key "$name.crt" "$name.key" "$name.ext"; do
  test -s "$certs/$file"
done
if git rev-parse --is-inside-work-tree >/dev/null 2>&1; then
  git diff --quiet HEAD -- "$manifest" "$config" || fail 'Commit or restore existing TLS manifest and allowlist changes before rotation.'
fi
kubectl --context "$context" -n "$namespace" get deployment/identity-api "deployment/$name-api" "secret/$secret" >/dev/null
mounted=$(kubectl --context "$context" -n "$namespace" get "secret/$secret" -o jsonpath='{.data.tls\.crt}')
local_cert=$(openssl base64 -A -in "$certs/$name.crt")
test "$mounted" = "$local_cert" || fail 'The local certificate does not match the current cluster Secret.'

rotation=$(mktemp -d "$certs/rotation.XXXXXX")
trap 'status=$?; if test "$status" -ne 0; then printf "Rotation stopped; inspect %s before retrying.\n" "$rotation" >&2; fi' EXIT
cp "$manifest" "$rotation/previous-sealed.yaml"

openssl genpkey -algorithm ED25519 -out "$rotation/new.key"
openssl req -new -key "$rotation/new.key" -out "$rotation/new.csr" -subj "/CN=$cn"
openssl x509 -req -in "$rotation/new.csr" -CA "$certs/ca.crt" -CAkey "$certs/ca.key" -CAcreateserial -out "$rotation/new.crt" -days 90 -extfile "$certs/$name.ext"
openssl verify -CAfile "$certs/ca.crt" -purpose "$purpose" "$rotation/new.crt"
kubeseal --controller-name sealed-secrets-controller --controller-namespace kube-system --fetch-cert > "$rotation/sealing.crt"
kubectl --context "$context" create secret generic "$secret" --namespace "$namespace" --from-file="tls.crt=$rotation/new.crt" --from-file="tls.key=$rotation/new.key" --from-file="ca.crt=$certs/ca.crt" --dry-run=client -o json > "$rotation/secret.json"
kubeseal --cert "$rotation/sealing.crt" --format yaml < "$rotation/secret.json" > "$rotation/sealed.yaml"
for item in ca.crt tls.crt tls.key; do
  grep -Fq "    $item:" "$rotation/sealed.yaml"
done
kubeseal --controller-name sealed-secrets-controller --controller-namespace kube-system --validate < "$rotation/sealed.yaml"

fingerprint() {
  openssl x509 -in "$1" -pubkey -noout > "$rotation/public.pem" || return 1
  openssl pkey -pubin -in "$rotation/public.pem" -outform DER -out "$rotation/public.der" || return 1
  digest=$(openssl dgst -sha256 -r "$rotation/public.der") || return 1
  fingerprint=${digest%% *}
  printf '%s\n' "$fingerprint" | grep -Eq '^[0-9a-f]{64}$' || return 1
  printf '%s\n' "$fingerprint"
}

read_allowlist() {
  awk -F'SESSION_CALLER_ALLOWLIST=' '/SESSION_CALLER_ALLOWLIST=/ { count++; print $2 } END { if (count != 1) exit 1 }' "$config"
}

replace_entry() {
  printf '%s\n' "$1" | awk -v old="$2" -v new="$3" '{ if (gsub(old, new) != 1) exit 1; print }'
}

set_allowlist() {
  value=$1
  printf '%s\n' "$value" | grep -Eq '^urn:flowspace:service:[A-Za-z0-9_-]+=[0-9a-f]{64}(,urn:flowspace:service:[A-Za-z0-9_-]+=[0-9a-f]{64})*$'
  awk -v value="$value" '/SESSION_CALLER_ALLOWLIST=/ { count++; sub(/SESSION_CALLER_ALLOWLIST=.*/, "SESSION_CALLER_ALLOWLIST=" value) } { print } END { if (count != 1) exit 1 }' "$config" > "$rotation/kustomization.yaml"
  mv "$rotation/kustomization.yaml" "$config"
  kubectl --context "$context" -n "$namespace" patch configmap identity-config --type merge -p "{\"data\":{\"SESSION_CALLER_ALLOWLIST\":\"$value\"}}"
}

rollout() {
  kubectl --context "$context" -n "$namespace" rollout restart "deployment/$1"
  kubectl --context "$context" -n "$namespace" rollout status "deployment/$1" --timeout=180s
}

if test "$name" = workspace; then
  current=$(read_allowlist)
  live=$(kubectl --context "$context" -n "$namespace" get configmap identity-config -o jsonpath='{.data.SESSION_CALLER_ALLOWLIST}')
  test "$current" = "$live"
  old_entry="urn:flowspace:service:workspace=$(fingerprint "$certs/workspace.crt")"
  new_entry="urn:flowspace:service:workspace=$(fingerprint "$rotation/new.crt")"
  overlap=$(replace_entry "$current" "$old_entry" "$old_entry,$new_entry")
  final=$(replace_entry "$overlap" "$old_entry," '')
  cp "$config" "$rotation/previous-kustomization.yaml"
  if test "$revoke_first" -eq 1; then set_allowlist "$final"; else set_allowlist "$overlap"; fi
  rollout identity-api
  sh scripts/smoke-identity-session-local.sh "$rotation/new.crt" "$rotation/new.key"
fi

mv "$rotation/sealed.yaml" "$manifest"
kubectl --context "$context" apply -f "$manifest"
expected=$(openssl base64 -A -in "$rotation/new.crt")
actual=
for attempt in 1 2 3 4 5 6 7 8 9 10 11 12 13 14 15 16 17 18 19 20 21 22 23 24 25 26 27 28 29 30; do
  actual=$(kubectl --context "$context" -n "$namespace" get "secret/$secret" -o jsonpath='{.data.tls\.crt}' 2>/dev/null || true)
  if test "$actual" = "$expected"; then break; fi
  sleep 1
done
test "$actual" = "$expected"
rollout "$name-api"
if test "$name" = workspace; then
  sh scripts/smoke-identity-session-local.sh "$rotation/new.crt" "$rotation/new.key"
else
  sh scripts/smoke-identity-session-local.sh
fi

cp "$certs/$name.crt" "$rotation/previous.crt"
cp "$certs/$name.key" "$rotation/previous.key"
mv "$rotation/new.crt" "$certs/$name.crt"
mv "$rotation/new.key" "$certs/$name.key"

if test "$name" = workspace; then
  if test "$revoke_first" -eq 0; then set_allowlist "$final"; fi
  rollout identity-api
  rollout workspace-api
  sh scripts/smoke-identity-session-local.sh
fi

printf '%s certificate rotated. Old key and manifest are in %s; review and commit the changed encrypted manifest.\n' "$name" "$rotation"
