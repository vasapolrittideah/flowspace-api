#!/bin/sh
set -eu

root=$(mktemp -d)
trap 'rm -rf "$root"' EXIT
mkdir -p "$root/scripts/local" "$root/bin" "$root/.secrets/identity-session"
cp scripts/local/setup-identity-session-tls-local.sh "$root/scripts/local/"
: > "$root/.secrets/identity-session/ca.key"
printf '#!/bin/sh\nprintf "k3d-flowspace\\n"\n' > "$root/bin/kubectl"
chmod +x "$root/bin/kubectl"

if PATH="$root/bin:$PATH" sh "$root/scripts/local/setup-identity-session-tls-local.sh" > "$root/output" 2>&1; then
  exit 1
fi
grep -q 'Refusing to replace .secrets/identity-session/ca.key' "$root/output"
