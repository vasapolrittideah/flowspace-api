#!/bin/sh
set -eu

cd "$(dirname "$0")/.."
umask 077
context=k3d-flowspace
namespace=flowspace-local
certs=.secrets/identity-session
config=deploy/overlays/local/identity/kustomization.yaml
test "$#" -eq 0
test "$(kubectl config current-context)" = "$context"
for file in ca.crt ca.key identity.crt identity.key identity.ext workspace.crt workspace.key workspace.ext unapproved.crt unapproved.key unapproved.ext; do
  test -s "$certs/$file"
done
if git rev-parse --is-inside-work-tree >/dev/null 2>&1; then
  git diff --quiet HEAD -- "$config" deploy/overlays/local/identity/session-tls-sealed-secret.yaml deploy/overlays/local/workspace/session-tls-sealed-secret.yaml || { printf 'Commit or restore existing TLS manifest and allowlist changes before rotation.\n' >&2; exit 1; }
fi
kubectl --context "$context" -n "$namespace" get deployment/identity-api deployment/workspace-api secret/identity-session-tls secret/workspace-session-tls >/dev/null
for name in identity workspace; do
  mounted=$(kubectl --context "$context" -n "$namespace" get "secret/$name-session-tls" -o jsonpath='{.data.tls\.crt}')
  local_cert=$(openssl base64 -A -in "$certs/$name.crt")
  test "$mounted" = "$local_cert" || { printf 'The local %s certificate does not match the current cluster Secret.\n' "$name" >&2; exit 1; }
done

rotation=$(mktemp -d "$certs/ca-rotation.XXXXXX")
trap 'status=$?; if test "$status" -ne 0; then printf "CA rotation stopped; inspect %s before retrying.\n" "$rotation" >&2; fi' EXIT
mkdir "$rotation/new"
cp "$config" "$rotation/previous-kustomization.yaml"
for name in identity workspace; do
  cp "deploy/overlays/local/$name/session-tls-sealed-secret.yaml" "$rotation/previous-$name-sealed.yaml"
done

openssl genpkey -algorithm ED25519 -out "$rotation/new/ca.key"
openssl req -new -x509 -key "$rotation/new/ca.key" -out "$rotation/new/ca.crt" -days 365 -subj '/CN=FlowSpace local session CA' -addext 'basicConstraints=critical,CA:TRUE,pathlen:0' -addext 'keyUsage=critical,keyCertSign,cRLSign'
issue_leaf() {
  name=$1
  openssl genpkey -algorithm ED25519 -out "$rotation/new/$name.key"
  openssl req -new -key "$rotation/new/$name.key" -out "$rotation/new/$name.csr" -subj "/CN=$2"
  openssl x509 -req -in "$rotation/new/$name.csr" -CA "$rotation/new/ca.crt" -CAkey "$rotation/new/ca.key" -CAcreateserial -out "$rotation/new/$name.crt" -days 90 -extfile "$certs/$name.ext"
  openssl verify -CAfile "$rotation/new/ca.crt" -purpose "$3" "$rotation/new/$name.crt"
}
issue_leaf identity identity-session sslserver
issue_leaf workspace workspace-api sslclient
issue_leaf unapproved other-api sslclient

fingerprint() {
  openssl x509 -in "$1" -pubkey -noout > "$rotation/public.pem" || return 1
  openssl pkey -pubin -in "$rotation/public.pem" -outform DER -out "$rotation/public.der" || return 1
  digest=$(openssl dgst -sha256 -r "$rotation/public.der") || return 1
  fingerprint=${digest%% *}
  printf '%s\n' "$fingerprint" | grep -Eq '^[0-9a-f]{64}$' || return 1
  printf '%s\n' "$fingerprint"
}
current=$(awk -F'SESSION_CALLER_ALLOWLIST=' '/SESSION_CALLER_ALLOWLIST=/ { count++; print $2 } END { if (count != 1) exit 1 }' "$config")
live=$(kubectl --context "$context" -n "$namespace" get configmap identity-config -o jsonpath='{.data.SESSION_CALLER_ALLOWLIST}')
test "$current" = "$live"
old_entry="urn:flowspace:service:workspace=$(fingerprint "$certs/workspace.crt")"
new_entry="urn:flowspace:service:workspace=$(fingerprint "$rotation/new/workspace.crt")"
updated=$(printf '%s\n' "$current" | awk -v old="$old_entry" -v new="$new_entry" '{ if (gsub(old, new) != 1) exit 1; print }')
printf '%s\n' "$updated" | grep -Eq '^urn:flowspace:service:[A-Za-z0-9_-]+=[0-9a-f]{64}(,urn:flowspace:service:[A-Za-z0-9_-]+=[0-9a-f]{64})*$'

kubeseal --controller-name sealed-secrets-controller --controller-namespace kube-system --fetch-cert > "$rotation/sealing.crt"
for name in identity workspace; do
  kubectl --context "$context" create secret generic "$name-session-tls" --namespace "$namespace" --from-file="tls.crt=$rotation/new/$name.crt" --from-file="tls.key=$rotation/new/$name.key" --from-file="ca.crt=$rotation/new/ca.crt" --dry-run=client -o json > "$rotation/$name-secret.json"
  kubeseal --cert "$rotation/sealing.crt" --format yaml < "$rotation/$name-secret.json" > "$rotation/$name-sealed.yaml"
  for item in ca.crt tls.crt tls.key; do grep -Fq "    $item:" "$rotation/$name-sealed.yaml"; done
  kubeseal --controller-name sealed-secrets-controller --controller-namespace kube-system --validate < "$rotation/$name-sealed.yaml"
done

for name in identity workspace; do
  manifest="deploy/overlays/local/$name/session-tls-sealed-secret.yaml"
  mv "$rotation/$name-sealed.yaml" "$manifest"
  kubectl --context "$context" apply -f "$manifest"
done
for name in identity workspace; do
  expected=$(openssl base64 -A -in "$rotation/new/$name.crt")
  actual=
  for attempt in 1 2 3 4 5 6 7 8 9 10 11 12 13 14 15 16 17 18 19 20 21 22 23 24 25 26 27 28 29 30; do
    actual=$(kubectl --context "$context" -n "$namespace" get "secret/$name-session-tls" -o jsonpath='{.data.tls\.crt}' 2>/dev/null || true)
    if test "$actual" = "$expected"; then break; fi
    sleep 1
  done
  test "$actual" = "$expected"
done

awk -v value="$updated" '/SESSION_CALLER_ALLOWLIST=/ { count++; sub(/SESSION_CALLER_ALLOWLIST=.*/, "SESSION_CALLER_ALLOWLIST=" value) } { print } END { if (count != 1) exit 1 }' "$config" > "$rotation/kustomization.yaml"
mv "$rotation/kustomization.yaml" "$config"
kubectl --context "$context" -n "$namespace" patch configmap identity-config --type merge -p "{\"data\":{\"SESSION_CALLER_ALLOWLIST\":\"$updated\"}}"
for name in identity workspace; do
  kubectl --context "$context" -n "$namespace" rollout restart "deployment/$name-api"
  kubectl --context "$context" -n "$namespace" rollout status "deployment/$name-api" --timeout=180s
done
sh scripts/smoke-identity-session-local.sh "$rotation/new/workspace.crt" "$rotation/new/workspace.key" "$rotation/new/ca.crt" "$rotation/new/unapproved.crt" "$rotation/new/unapproved.key"

for name in ca identity workspace unapproved; do
  cp "$certs/$name.crt" "$rotation/previous-$name.crt"
  cp "$certs/$name.key" "$rotation/previous-$name.key"
  mv "$rotation/new/$name.crt" "$certs/$name.crt"
  mv "$rotation/new/$name.key" "$certs/$name.key"
done
if test -f "$certs/ca.srl"; then mv "$certs/ca.srl" "$rotation/previous-ca.srl"; fi
mv "$rotation/new/ca.srl" "$certs/ca.srl"
sh scripts/smoke-identity-session-local.sh
printf 'Local session CA rotated. Old keys and manifests are in %s; review and commit both encrypted manifests and the allowlist.\n' "$rotation"
