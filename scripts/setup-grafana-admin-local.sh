#!/bin/sh
set -eu

cd "$(dirname "$0")/.."
umask 077
context=k3d-flowspace
password=.secrets/grafana-admin-password
manifest=deploy/overlays/local/observability/grafana-admin-sealed-secret.yaml
fail() { printf '%s\n' "$1" >&2; exit 1; }

test "$(kubectl config current-context)" = "$context" || fail 'Select the k3d-flowspace Kubernetes context.'
test -s "$password" || fail "Missing $password. Run task secrets:setup."

helm repo add sealed-secrets https://bitnami.github.io/sealed-secrets
helm upgrade --install sealed-secrets sealed-secrets/sealed-secrets --version 2.19.1 --namespace kube-system --kube-context "$context" --set-string fullnameOverride=sealed-secrets-controller --wait
kubeseal --controller-name sealed-secrets-controller --controller-namespace kube-system --fetch-cert > .secrets/sealing.crt

kubectl --context "$context" create secret generic grafana-admin --namespace flowspace-local --from-literal=admin-user=admin --from-file=admin-password="$password" --dry-run=client -o json | kubeseal --cert .secrets/sealing.crt --format yaml > .secrets/grafana-admin-sealed.yaml
kubeseal --controller-name sealed-secrets-controller --controller-namespace kube-system --validate < .secrets/grafana-admin-sealed.yaml

mkdir -p "$(dirname "$manifest")"
mv .secrets/grafana-admin-sealed.yaml "$manifest"
printf 'The Grafana admin password is sealed. Review and commit %s.\n' "$manifest"
