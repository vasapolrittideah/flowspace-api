#!/bin/sh
set -eu

cd "$(dirname "$0")/.."
umask 077
certs=.secrets/identity-session
test "$(kubectl config current-context)" = k3d-flowspace
mkdir -p "$certs"
for key in ca identity workspace unapproved; do
  if test -e "$certs/$key.key" || test -L "$certs/$key.key"; then
    printf 'Refusing to replace %s/%s.key\n' "$certs" "$key" >&2
    exit 1
  fi
done

helm repo add sealed-secrets https://bitnami.github.io/sealed-secrets
helm upgrade --install sealed-secrets sealed-secrets/sealed-secrets --version 2.19.1 --namespace kube-system --kube-context k3d-flowspace --set-string fullnameOverride=sealed-secrets-controller --wait
kubeseal --controller-name sealed-secrets-controller --controller-namespace kube-system --fetch-cert > "$certs/sealing.crt"

openssl genpkey -algorithm ED25519 -out "$certs/ca.key"
openssl req -new -x509 -key "$certs/ca.key" -out "$certs/ca.crt" -days 365 -subj '/CN=FlowSpace local session CA' -addext 'basicConstraints=critical,CA:TRUE,pathlen:0' -addext 'keyUsage=critical,keyCertSign,cRLSign'

issue_leaf() {
  name=$1
  openssl genpkey -algorithm ED25519 -out "$certs/$name.key"
  openssl req -new -key "$certs/$name.key" -out "$certs/$name.csr" -subj "/CN=$2"
  printf 'basicConstraints=critical,CA:FALSE\nkeyUsage=critical,digitalSignature\nextendedKeyUsage=%s\nsubjectAltName=%s\n' "$3" "$4" > "$certs/$name.ext"
  openssl x509 -req -in "$certs/$name.csr" -CA "$certs/ca.crt" -CAkey "$certs/ca.key" -CAcreateserial -out "$certs/$name.crt" -days 90 -extfile "$certs/$name.ext"
}

issue_leaf identity identity-session serverAuth 'DNS:identity-session.flowspace-local.svc,DNS:identity-session.flowspace-local.svc.cluster.local'
issue_leaf workspace workspace-api clientAuth 'URI:urn:flowspace:service:workspace'
issue_leaf unapproved other-api clientAuth 'URI:urn:flowspace:service:other'

kubectl create secret generic identity-session-tls --namespace flowspace-local --from-file="tls.crt=$certs/identity.crt" --from-file="tls.key=$certs/identity.key" --from-file="ca.crt=$certs/ca.crt" --dry-run=client -o json | kubeseal --cert "$certs/sealing.crt" --format yaml > "$certs/identity-sealed.yaml"
kubectl create secret generic workspace-session-tls --namespace flowspace-local --from-file="tls.crt=$certs/workspace.crt" --from-file="tls.key=$certs/workspace.key" --from-file="ca.crt=$certs/ca.crt" --dry-run=client -o json | kubeseal --cert "$certs/sealing.crt" --format yaml > "$certs/workspace-sealed.yaml"
kubeseal --controller-name sealed-secrets-controller --controller-namespace kube-system --validate < "$certs/identity-sealed.yaml"
kubeseal --controller-name sealed-secrets-controller --controller-namespace kube-system --validate < "$certs/workspace-sealed.yaml"

fingerprint=$(openssl x509 -in "$certs/workspace.crt" -pubkey -noout | openssl pkey -pubin -outform DER | openssl dgst -sha256 -r)
fingerprint=${fingerprint%% *}
config=deploy/overlays/local/identity/kustomization.yaml
awk -v fingerprint="$fingerprint" '/SESSION_CALLER_ALLOWLIST=/ { count++; sub(/SESSION_CALLER_ALLOWLIST=.*/, "SESSION_CALLER_ALLOWLIST=urn:flowspace:service:workspace=" fingerprint) } { print } END { if (count != 1) exit 1 }' "$config" > "$certs/kustomization.yaml"
mv "$certs/kustomization.yaml" "$config"
mv "$certs/identity-sealed.yaml" deploy/overlays/local/identity/session-tls-sealed-secret.yaml
mv "$certs/workspace-sealed.yaml" deploy/overlays/local/workspace/session-tls-sealed-secret.yaml
printf 'Local session TLS is sealed. Run tilt up, task secrets, and sh scripts/smoke-identity-session-local.sh.\n'
