#!/bin/sh
set -eu

root=$(mktemp -d)
trap 'rm -rf "$root"' EXIT
mkdir -p "$root/scripts" "$root/bin" "$root/state" "$root/.secrets/identity-session" "$root/deploy/overlays/local/identity" "$root/deploy/overlays/local/workspace"
cp scripts/rotate-identity-session-tls-local.sh "$root/scripts/"
cp scripts/rotate-identity-session-ca-local.sh "$root/scripts/"
certs="$root/.secrets/identity-session"
openssl genpkey -algorithm ED25519 -out "$certs/ca.key"
openssl req -new -x509 -key "$certs/ca.key" -out "$certs/ca.crt" -days 365 -subj '/CN=Test CA' -addext 'basicConstraints=critical,CA:TRUE,pathlen:0' -addext 'keyUsage=critical,keyCertSign,cRLSign'
for name in identity workspace unapproved; do
  case "$name" in
    identity) cn=identity-session; usage=serverAuth; san=DNS:identity-session.flowspace-local.svc ;;
    workspace) cn=workspace-api; usage=clientAuth; san=URI:urn:flowspace:service:workspace ;;
    unapproved) cn=other-api; usage=clientAuth; san=URI:urn:flowspace:service:other ;;
  esac
  openssl genpkey -algorithm ED25519 -out "$certs/$name.key"
  openssl req -new -key "$certs/$name.key" -out "$certs/$name.csr" -subj "/CN=$cn"
  printf 'basicConstraints=critical,CA:FALSE\nkeyUsage=critical,digitalSignature\nextendedKeyUsage=%s\nsubjectAltName=%s\n' "$usage" "$san" > "$certs/$name.ext"
  openssl x509 -req -in "$certs/$name.csr" -CA "$certs/ca.crt" -CAkey "$certs/ca.key" -CAcreateserial -out "$certs/$name.crt" -days 90 -extfile "$certs/$name.ext" >/dev/null 2>&1
  if test "$name" != unapproved; then printf 'previous manifest\n' > "$root/deploy/overlays/local/$name/session-tls-sealed-secret.yaml"; fi
done

openssl x509 -in "$certs/workspace.crt" -pubkey -noout > "$root/state/public.pem"
openssl pkey -pubin -in "$root/state/public.pem" -outform DER -out "$root/state/public.der"
digest=$(openssl dgst -sha256 -r "$root/state/public.der")
old=${digest%% *}
printf '  - SESSION_CALLER_ALLOWLIST=urn:flowspace:service:workspace=%s\n' "$old" > "$root/deploy/overlays/local/identity/kustomization.yaml"
printf 'urn:flowspace:service:workspace=%s' "$old" > "$root/state/allowlist"
openssl base64 -A -in "$certs/identity.crt" > "$root/state/identity-cert"
openssl base64 -A -in "$certs/workspace.crt" > "$root/state/workspace-cert"
export FLOWSPACE_TLS_TEST_STATE="$root/state"

cat > "$root/bin/kubectl" <<'SH'
#!/bin/sh
set -eu
state=$FLOWSPACE_TLS_TEST_STATE
while test "$#" -gt 0; do
  case "$1" in --context|-n|--namespace) shift 2 ;; *) break ;; esac
done
case "$1" in
  config) printf 'k3d-flowspace\n' ;;
  get)
    case "$2" in
      configmap) cat "$state/allowlist" ;;
      secret/*) name=${2#secret/}; cat "$state/${name%-session-tls}-cert" ;;
    esac ;;
  create) test "${FLOWSPACE_TLS_TEST_FAIL_CREATE:-0}" -eq 0; printf '{}\n' ;;
  patch)
    while test "$#" -gt 0 && test "$1" != -p; do shift; done
    printf '%s' "$2" | sed 's/.*SESSION_CALLER_ALLOWLIST":"\([^"]*\)".*/\1/' > "$state/allowlist"
    printf 'patch\n' >> "$state/log" ;;
  apply)
    case "$3" in *identity/*) name=identity ;; *workspace/*) name=workspace ;; esac
    for cert in .secrets/identity-session/rotation.*/new.crt .secrets/identity-session/ca-rotation.*/new/"$name.crt"; do
      if test -f "$cert"; then openssl base64 -A -in "$cert" > "$state/$name-cert"; fi
    done
    printf 'apply\n' >> "$state/log" ;;
  rollout) printf 'rollout %s %s\n' "$2" "$3" >> "$state/log" ;;
esac
SH
cat > "$root/bin/kubeseal" <<'SH'
#!/bin/sh
case " $* " in
  *' --fetch-cert '*) printf 'test certificate\n' ;;
  *' --validate '*) cat >/dev/null; printf 'validate\n' >> "$FLOWSPACE_TLS_TEST_STATE/log" ;;
  *) cat >/dev/null; printf 'kind: SealedSecret\nspec:\n  encryptedData:\n    ca.crt: sealed\n    tls.crt: sealed\n    tls.key: sealed\n' ;;
esac
SH
cat > "$root/scripts/smoke-identity-session-local.sh" <<'SH'
#!/bin/sh
for file in "$@"; do test -s "$file"; done
printf 'smoke %s\n' "$#" >> "$FLOWSPACE_TLS_TEST_STATE/log"
SH
chmod +x "$root/bin/kubectl" "$root/bin/kubeseal"

PATH="$root/bin:$PATH" sh "$root/scripts/rotate-identity-session-tls-local.sh" identity
PATH="$root/bin:$PATH" sh "$root/scripts/rotate-identity-session-tls-local.sh" workspace
PATH="$root/bin:$PATH" sh "$root/scripts/rotate-identity-session-ca-local.sh"
PATH="$root/bin:$PATH" sh "$root/scripts/rotate-identity-session-tls-local.sh" workspace-revoke
test "$(find "$certs" -name previous.key | wc -l | tr -d ' ')" = 3
test "$(find "$certs" -name 'previous-*.key' | wc -l | tr -d ' ')" = 4
test "$(grep -c '^patch$' "$root/state/log")" = 4
test "$(grep -c '^smoke 2$' "$root/state/log")" = 4
test "$(grep -c '^smoke 5$' "$root/state/log")" = 1
test "$(grep -c '^smoke 0$' "$root/state/log")" = 4
if grep -q "$old" "$root/deploy/overlays/local/identity/kustomization.yaml"; then exit 1; fi
test "$(cat "$root/state/allowlist")" = "$(sed -n 's/.*SESSION_CALLER_ALLOWLIST=//p' "$root/deploy/overlays/local/identity/kustomization.yaml")"

cat > "$root/state/expected" <<'LOG'
validate
apply
rollout restart deployment/identity-api
rollout status deployment/identity-api
smoke 0
validate
patch
rollout restart deployment/identity-api
rollout status deployment/identity-api
smoke 2
apply
rollout restart deployment/workspace-api
rollout status deployment/workspace-api
smoke 2
patch
rollout restart deployment/identity-api
rollout status deployment/identity-api
rollout restart deployment/workspace-api
rollout status deployment/workspace-api
smoke 0
validate
validate
apply
apply
patch
rollout restart deployment/identity-api
rollout status deployment/identity-api
rollout restart deployment/workspace-api
rollout status deployment/workspace-api
smoke 5
smoke 0
validate
patch
rollout restart deployment/identity-api
rollout status deployment/identity-api
smoke 2
apply
rollout restart deployment/workspace-api
rollout status deployment/workspace-api
smoke 2
rollout restart deployment/identity-api
rollout status deployment/identity-api
rollout restart deployment/workspace-api
rollout status deployment/workspace-api
smoke 0
LOG
diff -u "$root/state/expected" "$root/state/log"

printf 'stale' > "$root/state/workspace-cert"
if PATH="$root/bin:$PATH" sh "$root/scripts/rotate-identity-session-tls-local.sh" workspace > "$root/state/error" 2>&1; then exit 1; fi
grep -q 'local certificate does not match' "$root/state/error"

openssl base64 -A -in "$certs/workspace.crt" > "$root/state/workspace-cert"
if FLOWSPACE_TLS_TEST_FAIL_CREATE=1 PATH="$root/bin:$PATH" sh "$root/scripts/rotate-identity-session-tls-local.sh" identity > "$root/state/error" 2>&1; then exit 1; fi
test "$(grep -c '^apply$' "$root/state/log")" = 5
