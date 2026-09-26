#!/bin/sh
set -eu

root=$(mktemp -d)
trap 'rm -rf "$root"' EXIT
mkdir -p "$root/scripts" "$root/bin" "$root/.secrets" "$root/deploy/overlays/local/identity"
cp scripts/retire-identity-signing-key-local.sh "$root/scripts/"
config="$root/deploy/overlays/local/identity/kustomization.yaml"
printf '      - SIGNING_KEY_ID=new\n      - SIGNING_ADDITIONAL_PUBLIC_KEY=old:public\n' > "$config"
cat > "$root/bin/date" <<'SH'
#!/bin/sh
printf '1000\n'
SH
chmod +x "$root/bin/date"

printf 'retired:310\n' > "$root/.secrets/identity-signing-old-key-last-use"
printf '      - SIGNING_KEY_ID=old\n      - SIGNING_ADDITIONAL_PUBLIC_KEY=new:public\n' > "$config"
if PATH="$root/bin:$PATH" sh "$root/scripts/retire-identity-signing-key-local.sh" >/dev/null 2>&1; then exit 1; fi
grep -q 'SIGNING_ADDITIONAL_PUBLIC_KEY=new:public' "$config"
printf '      - SIGNING_KEY_ID=new\n      - SIGNING_ADDITIONAL_PUBLIC_KEY=old:public\n' > "$config"

printf 'old:311\n' > "$root/.secrets/identity-signing-old-key-last-use"
if PATH="$root/bin:$PATH" sh "$root/scripts/retire-identity-signing-key-local.sh" >/dev/null 2>&1; then exit 1; fi
grep -q 'SIGNING_ADDITIONAL_PUBLIC_KEY=old:public' "$config"

printf 'old:310\n' > "$root/.secrets/identity-signing-old-key-last-use"
PATH="$root/bin:$PATH" sh "$root/scripts/retire-identity-signing-key-local.sh"
grep -q '^      - SIGNING_ADDITIONAL_PUBLIC_KEY=$' "$config"
