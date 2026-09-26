#!/bin/sh
set -eu

cd "$(dirname "$0")/.."
test "$#" -eq 0
config=deploy/overlays/local/identity/kustomization.yaml
state=.secrets/identity-signing-old-key-last-use
record=$(cat "$state")
old_id=${record%%:*}
last_use=${record#*:}
printf '%s\n' "$old_id" | grep -Eq '^[A-Za-z0-9_-]{1,64}$' || { printf 'Invalid old-key record.\n' >&2; exit 1; }
printf '%s\n' "$last_use" | grep -Eq '^[1-9][0-9]{0,11}$' || { printf 'Invalid old-key last-use time.\n' >&2; exit 1; }
active=$(awk -F= '/SIGNING_KEY_ID=/ { print $2 }' "$config")
published=$(awk -F'[:=]' '/SIGNING_ADDITIONAL_PUBLIC_KEY=/ { print $2 }' "$config")
if test "$published" != "$old_id" || test "$active" = "$old_id"; then
  printf 'The recorded old key does not match the published key.\n' >&2
  exit 1
fi
now=$(date -u +%s)
if test "$now" -lt "$((last_use + 690))"; then
  printf 'Keep the old public key for at least 690 seconds after its last possible use.\n' >&2
  exit 1
fi

temporary=$(mktemp "$config.XXXXXX")
trap 'rm -f "$temporary"' EXIT
awk '/^[[:space:]]*- SIGNING_ADDITIONAL_PUBLIC_KEY=/ {
  count++
  if ($0 ~ /=$/) exit 1
  sub(/=.*/, "=")
} { print } END { if (count != 1) exit 1 }' "$config" > "$temporary"
mv "$temporary" "$config"
printf 'Old public key removed from the local overlay. Let Tilt apply it, restart identity-api, and check JWKS.\n'
