#!/bin/sh
set -eu

root=$(mktemp -d)
trap 'rm -rf "$root"' EXIT
mkdir -p "$root/scripts" "$root/bin" "$root/state" "$root/.secrets"
cp scripts/setup-grafana-admin-local.sh "$root/scripts/"
manifest="$root/deploy/overlays/local/observability/grafana-admin-sealed-secret.yaml"
export FLOWSPACE_GRAFANA_TEST_ROOT="$root"

cat > "$root/bin/kubectl" <<'SH'
#!/bin/sh
set -eu
if test "$1" = config; then
  printf '%s\n' "${FLOWSPACE_GRAFANA_TEST_CONTEXT:-k3d-flowspace}"
  exit 0
fi
printf '%s\n' "$*" >> "$FLOWSPACE_GRAFANA_TEST_ROOT/state/kubectl"
printf '{"kind":"Secret","metadata":{"name":"grafana-admin"}}\n'
SH
cat > "$root/bin/helm" <<'SH'
#!/bin/sh
printf '%s\n' "$*" >> "$FLOWSPACE_GRAFANA_TEST_ROOT/state/helm"
SH
cat > "$root/bin/kubeseal" <<'SH'
#!/bin/sh
set -eu
case " $* " in
  *' --fetch-cert '*) printf 'controller certificate\n' ;;
  *' --validate '*) cat > /dev/null; test "${FLOWSPACE_GRAFANA_TEST_FAIL_VALIDATE:-0}" -eq 0 ;;
  *' --format yaml '*)
    grep -q grafana-admin
    printf 'kind: SealedSecret\nmetadata:\n  name: grafana-admin\nspec:\n  encryptedData:\n    admin-password: encrypted\n    admin-user: encrypted\n' ;;
  *) exit 1 ;;
esac
SH
chmod +x "$root/bin/kubectl" "$root/bin/helm" "$root/bin/kubeseal"

# The script refuses to run without a password file.
if PATH="$root/bin:$PATH" sh "$root/scripts/setup-grafana-admin-local.sh" > "$root/state/output" 2>&1; then
  exit 1
fi
grep -q 'Missing .secrets/grafana-admin-password' "$root/state/output"
test ! -e "$manifest"

# The script seals the password file into the observability overlay.
printf 'local-test-password' > "$root/.secrets/grafana-admin-password"
PATH="$root/bin:$PATH" sh "$root/scripts/setup-grafana-admin-local.sh" > "$root/state/output"
grep -q 'name: grafana-admin' "$manifest"
grep -q 'admin-password: encrypted' "$manifest"
grep -q -- '--version 2.19.1' "$root/state/helm"
grep -q -- '--from-file=admin-password=.secrets/grafana-admin-password' "$root/state/kubectl"
if grep -q 'local-test-password' "$root/state/kubectl" "$root/state/output" "$manifest"; then
  exit 1
fi

# A failed validation keeps the previous manifest.
printf 'previous manifest\n' > "$manifest"
if FLOWSPACE_GRAFANA_TEST_FAIL_VALIDATE=1 PATH="$root/bin:$PATH" sh "$root/scripts/setup-grafana-admin-local.sh" > "$root/state/output" 2>&1; then
  exit 1
fi
test "$(cat "$manifest")" = 'previous manifest'

# The script refuses another Kubernetes context.
if FLOWSPACE_GRAFANA_TEST_CONTEXT=k3d-other PATH="$root/bin:$PATH" sh "$root/scripts/setup-grafana-admin-local.sh" > "$root/state/output" 2>&1; then
  exit 1
fi
grep -q 'Select the k3d-flowspace Kubernetes context.' "$root/state/output"
