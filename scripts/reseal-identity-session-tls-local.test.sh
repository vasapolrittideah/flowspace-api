#!/bin/sh
set -eu

root=$(mktemp -d)
trap 'rm -rf "$root"' EXIT
mkdir -p "$root/scripts" "$root/bin" "$root/state" "$root/.secrets/identity-session" "$root/deploy/overlays/local/identity" "$root/deploy/overlays/local/workspace"
cp scripts/reseal-identity-session-tls-local.sh "$root/scripts/"
certs="$root/.secrets/identity-session"
openssl genpkey -algorithm ED25519 -out "$certs/ca.key"
openssl req -new -x509 -key "$certs/ca.key" -out "$certs/ca.crt" -days 365 -subj '/CN=Test CA' -addext 'basicConstraints=critical,CA:TRUE,pathlen:0' -addext 'keyUsage=critical,keyCertSign,cRLSign'
for name in identity workspace; do
  case "$name" in
    identity) cn=identity-session; usage=serverAuth; san=DNS:identity-session.flowspace-local.svc ;;
    workspace) cn=workspace-api; usage=clientAuth; san=URI:urn:flowspace:service:workspace ;;
  esac
  openssl genpkey -algorithm ED25519 -out "$certs/$name.key"
  openssl req -new -key "$certs/$name.key" -out "$certs/$name.csr" -subj "/CN=$cn"
  printf 'basicConstraints=critical,CA:FALSE\nkeyUsage=critical,digitalSignature\nextendedKeyUsage=%s\nsubjectAltName=%s\n' "$usage" "$san" > "$certs/$name.ext"
  openssl x509 -req -in "$certs/$name.csr" -CA "$certs/ca.crt" -CAkey "$certs/ca.key" -CAcreateserial -out "$certs/$name.crt" -days 90 -extfile "$certs/$name.ext" >/dev/null 2>&1
  printf 'previous manifest\n' > "$root/deploy/overlays/local/$name/session-tls-sealed-secret.yaml"
done
fingerprint=$(openssl x509 -in "$certs/workspace.crt" -pubkey -noout | openssl pkey -pubin -outform DER | openssl dgst -sha256 -r)
fingerprint=${fingerprint%% *}
printf '  - SESSION_CALLER_ALLOWLIST=urn:flowspace:service:workspace=%s\n' "$fingerprint" > "$root/deploy/overlays/local/identity/kustomization.yaml"
export FLOWSPACE_RESEAL_TEST_ROOT="$root"

cat > "$root/bin/kubectl" <<'SH'
#!/bin/sh
set -eu
if test "$1" = config; then
  printf '%s\n' "${FLOWSPACE_RESEAL_TEST_CONTEXT:-k3d-flowspace}"
  exit 0
fi
printf '%s\n' "$*" >> "$FLOWSPACE_RESEAL_TEST_ROOT/state/kubectl"
while test "$1" != generic; do shift; done
shift
printf '{"kind":"Secret","metadata":{"name":"%s"}}\n' "$1"
SH
cat > "$root/bin/helm" <<'SH'
#!/bin/sh
printf '%s\n' "$*" >> "$FLOWSPACE_RESEAL_TEST_ROOT/state/helm"
SH
cat > "$root/bin/kubeseal" <<'SH'
#!/bin/sh
set -eu
case " $* " in
  *' --fetch-cert '*) printf 'new controller certificate\n' ;;
  *' --validate '*)
    input=$(cat)
    case "$input" in
      *workspace-session-tls*) test "${FLOWSPACE_RESEAL_TEST_FAIL_WORKSPACE:-0}" -eq 0 ;;
    esac ;;
  *' --format yaml '*)
    input=$(cat)
    case "$input" in
      *identity-session-tls*) name=identity ;;
      *workspace-session-tls*) name=workspace ;;
      *) exit 1 ;;
    esac
    printf 'kind: SealedSecret\nmetadata:\n  name: %s-session-tls\nspec:\n  encryptedData:\n    ca.crt: encrypted-%s\n    tls.crt: encrypted-%s\n    tls.key: encrypted-%s\n' "$name" "$name" "$name" "$name" ;;
  *) exit 1 ;;
esac
SH
chmod +x "$root/bin/kubectl" "$root/bin/helm" "$root/bin/kubeseal"

PATH="$root/bin:$PATH" sh "$root/scripts/reseal-identity-session-tls-local.sh" > "$root/state/output"
for name in identity workspace; do
  grep -q "name: $name-session-tls" "$root/deploy/overlays/local/$name/session-tls-sealed-secret.yaml"
  grep -q "tls.key: encrypted-$name" "$root/deploy/overlays/local/$name/session-tls-sealed-secret.yaml"
done
grep -q -- '--version 2.19.1' "$root/state/helm"

for name in identity workspace; do
  printf 'previous manifest\n' > "$root/deploy/overlays/local/$name/session-tls-sealed-secret.yaml"
done
if FLOWSPACE_RESEAL_TEST_FAIL_WORKSPACE=1 PATH="$root/bin:$PATH" sh "$root/scripts/reseal-identity-session-tls-local.sh" > "$root/state/output" 2>&1; then
  exit 1
fi
for name in identity workspace; do
  test "$(cat "$root/deploy/overlays/local/$name/session-tls-sealed-secret.yaml")" = 'previous manifest'
done

if FLOWSPACE_RESEAL_TEST_CONTEXT=k3d-other PATH="$root/bin:$PATH" sh "$root/scripts/reseal-identity-session-tls-local.sh" > "$root/state/output" 2>&1; then
  exit 1
fi
