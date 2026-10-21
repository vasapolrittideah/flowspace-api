#!/bin/sh
set -eu

success=$(mktemp -d)
trap 'rm -rf "$success"' EXIT
mkdir -p "$success/scripts/local" "$success/bin" "$success/deploy/overlays/local/identity" "$success/deploy/overlays/local/workspace"
cp scripts/local/setup-identity-session-tls-local.sh "$success/scripts/local/"
printf '  - SESSION_CALLER_ALLOWLIST=old\n' > "$success/deploy/overlays/local/identity/kustomization.yaml"
cat > "$success/bin/kubectl" <<'SH'
#!/bin/sh
case "$1" in
  config) printf 'k3d-flowspace\n' ;;
  create) printf '{}\n' ;;
esac
SH
printf '#!/bin/sh\n' > "$success/bin/helm"
cat > "$success/bin/kubeseal" <<'SH'
#!/bin/sh
case " $* " in
  *' --fetch-cert '*) printf 'test certificate\n' ;;
  *) cat >/dev/null; printf 'kind: SealedSecret\n' ;;
esac
SH
chmod +x "$success/bin/kubectl" "$success/bin/helm" "$success/bin/kubeseal"

PATH="$success/bin:$PATH" sh "$success/scripts/local/setup-identity-session-tls-local.sh" > "$success/output" 2>/dev/null
command=$(sed -n 's/.*and sh \([^ ]*\)\.$/\1/p' "$success/output")
test "$command" = scripts/local/smoke-identity-session-local.sh
test -f "$command"
grep -q 'SESSION_CALLER_ALLOWLIST=urn:flowspace:service:workspace=' "$success/deploy/overlays/local/identity/kustomization.yaml"
