#!/bin/sh
set -eu

cd "$(dirname "$0")/.."
umask 077
context=k3d-flowspace
certs=.secrets/identity-session
config=deploy/overlays/local/identity/kustomization.yaml
fail() { printf '%s\n' "$1" >&2; exit 1; }

test "$(kubectl config current-context)" = "$context" || fail 'Select the k3d-flowspace Kubernetes context.'
for file in ca.crt identity.crt identity.key workspace.crt workspace.key; do
  test -s "$certs/$file" || fail "Missing $certs/$file"
done
for name in identity workspace; do
  case "$name" in identity) purpose=sslserver ;; workspace) purpose=sslclient ;; esac
  openssl verify -CAfile "$certs/ca.crt" -purpose "$purpose" "$certs/$name.crt"
  cert_public=$(openssl x509 -in "$certs/$name.crt" -pubkey -noout)
  key_public=$(openssl pkey -in "$certs/$name.key" -pubout)
  test "$cert_public" = "$key_public" || fail "The $name certificate and private key do not match."
done
fingerprint=$(openssl x509 -in "$certs/workspace.crt" -pubkey -noout | openssl pkey -pubin -outform DER | openssl dgst -sha256 -r)
fingerprint=${fingerprint%% *}
allowlist=$(sed -n 's/.*SESSION_CALLER_ALLOWLIST=//p' "$config")
test "$allowlist" = "urn:flowspace:service:workspace=$fingerprint" || fail 'The Workspace certificate does not match the Identity allowlist.'
if git rev-parse --is-inside-work-tree >/dev/null 2>&1; then
  git diff --quiet HEAD -- "$config" deploy/overlays/local/identity/session-tls-sealed-secret.yaml deploy/overlays/local/workspace/session-tls-sealed-secret.yaml || fail 'Commit or restore existing TLS manifest and allowlist changes before resealing.'
fi

helm repo add sealed-secrets https://bitnami.github.io/sealed-secrets
helm upgrade --install sealed-secrets sealed-secrets/sealed-secrets --version 2.19.1 --namespace kube-system --kube-context "$context" --set-string fullnameOverride=sealed-secrets-controller --wait
kubeseal --controller-name sealed-secrets-controller --controller-namespace kube-system --fetch-cert > "$certs/sealing.crt"

for name in identity workspace; do
  kubectl --context "$context" create secret generic "$name-session-tls" --namespace flowspace-local --from-file="tls.crt=$certs/$name.crt" --from-file="tls.key=$certs/$name.key" --from-file="ca.crt=$certs/ca.crt" --dry-run=client -o json | kubeseal --cert "$certs/sealing.crt" --format yaml > "$certs/$name-sealed.yaml"
  kubeseal --controller-name sealed-secrets-controller --controller-namespace kube-system --validate < "$certs/$name-sealed.yaml"
done

mv "$certs/identity-sealed.yaml" deploy/overlays/local/identity/session-tls-sealed-secret.yaml
mv "$certs/workspace-sealed.yaml" deploy/overlays/local/workspace/session-tls-sealed-secret.yaml
printf 'Local session TLS is resealed. Run tilt up, then review and commit both manifests.\n'
