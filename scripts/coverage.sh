#!/bin/sh
set -e

profile="$(mktemp)"
trap 'rm -f "$profile"' EXIT
go test ./... -coverprofile="$profile"
node scripts/constraint-check.mjs coverage --profile "$profile" --changed-min 80 --total-min 25
