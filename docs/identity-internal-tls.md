# Local Identity session TLS

The local Identity API serves `CheckSession` at `identity-session.flowspace-local.svc:8082`. The `identity-session` Service is a cluster-only `ClusterIP` Service. The local NetworkPolicy admits port 8082 only from pods labeled `app.kubernetes.io/name: workspace-api` in `flowspace-local`. Identity still serves public requests on port 8080. Its existing health and JWKS listener stays on port 8081 and is available to Workspace through the separate `identity-internal` ClusterIP Service.

Identity requires TLS 1.3 and a client certificate signed by the local CA. The certificate must have one URI subject alternative name, `urn:flowspace:service:workspace`, and a public-key fingerprint in `SESSION_CALLER_ALLOWLIST`. Workspace must validate Identity's server certificate with the local CA and the Service DNS name. Workspace can fetch signing keys at `http://identity-internal.flowspace-local.svc:8081/.well-known/jwks.json`. The current Workspace API does not call `CheckSession`; Issue #116 will use the mounted client certificate and the private Service.

## Issue local certificates

Use the `k3d-flowspace` context with `helm`, `kubeseal`, `openssl`, `kubectl`, Tilt, and `grpcurl` installed. Keep all private keys in `.secrets/identity-session/`, which Git and Docker ignore. Back up the CA key and the Sealed Secrets controller keys outside Git before relying on the encrypted manifests after a cluster rebuild. The CA private key must never enter Kubernetes.

Install the pinned controller before sealing. Tilt also manages this chart when it starts.

```sh
umask 077
mkdir -p .secrets/identity-session
helm repo add sealed-secrets https://bitnami.github.io/sealed-secrets
helm upgrade --install sealed-secrets sealed-secrets/sealed-secrets --version 2.19.1 --namespace kube-system --set-string fullnameOverride=sealed-secrets-controller --wait
kubeseal --controller-name sealed-secrets-controller --controller-namespace kube-system --fetch-cert > .secrets/identity-session/sealing.crt
```

For a new local CA and leaf certificates, run these commands from the repository root. Stop if `.secrets/identity-session/ca.key` already exists; the commands create a new trust root. The extension files stay outside Git with the keys.

```sh
umask 077
mkdir -p .secrets/identity-session
cd .secrets/identity-session
test ! -e ca.key
openssl genpkey -algorithm ED25519 -out ca.key
openssl req -new -x509 -key ca.key -out ca.crt -days 365 -subj '/CN=FlowSpace local session CA' -addext 'basicConstraints=critical,CA:TRUE,pathlen:0' -addext 'keyUsage=critical,keyCertSign,cRLSign'
openssl genpkey -algorithm ED25519 -out identity.key
openssl req -new -key identity.key -out identity.csr -subj '/CN=identity-session'
printf 'basicConstraints=critical,CA:FALSE\nkeyUsage=critical,digitalSignature\nextendedKeyUsage=serverAuth\nsubjectAltName=DNS:identity-session.flowspace-local.svc,DNS:identity-session.flowspace-local.svc.cluster.local\n' > identity.ext
openssl x509 -req -in identity.csr -CA ca.crt -CAkey ca.key -CAcreateserial -out identity.crt -days 90 -extfile identity.ext
openssl genpkey -algorithm ED25519 -out workspace.key
openssl req -new -key workspace.key -out workspace.csr -subj '/CN=workspace-api'
printf 'basicConstraints=critical,CA:FALSE\nkeyUsage=critical,digitalSignature\nextendedKeyUsage=clientAuth\nsubjectAltName=URI:urn:flowspace:service:workspace\n' > workspace.ext
openssl x509 -req -in workspace.csr -CA ca.crt -CAkey ca.key -CAcreateserial -out workspace.crt -days 90 -extfile workspace.ext
openssl genpkey -algorithm ED25519 -out unapproved.key
openssl req -new -key unapproved.key -out unapproved.csr -subj '/CN=other-api'
printf 'basicConstraints=critical,CA:FALSE\nkeyUsage=critical,digitalSignature\nextendedKeyUsage=clientAuth\nsubjectAltName=URI:urn:flowspace:service:other\n' > unapproved.ext
openssl x509 -req -in unapproved.csr -CA ca.crt -CAkey ca.key -CAcreateserial -out unapproved.crt -days 90 -extfile unapproved.ext
```

Compute the fingerprint from the Workspace certificate's `SubjectPublicKeyInfo` bytes. Put the 64 hexadecimal characters after `urn:flowspace:service:workspace=` in `deploy/overlays/local/identity/kustomization.yaml`.

```sh
openssl x509 -in .secrets/identity-session/workspace.crt -pubkey -noout | openssl pkey -pubin -outform DER | openssl dgst -sha256 -r
```

## Seal and deploy

Run these commands from the repository root. The plaintext Secret manifests pass through a pipe and never go into a file. Sealed Secrets use strict name and namespace binding. The server and client keys go into different workload Secrets. Each Secret includes the public CA certificate so the workload can verify its peer.

```sh
kubectl create secret generic identity-session-tls --namespace flowspace-local --from-file=tls.crt=.secrets/identity-session/identity.crt --from-file=tls.key=.secrets/identity-session/identity.key --from-file=ca.crt=.secrets/identity-session/ca.crt --dry-run=client -o json | kubeseal --cert .secrets/identity-session/sealing.crt --format yaml > deploy/overlays/local/identity/session-tls-sealed-secret.yaml
kubectl create secret generic workspace-session-tls --namespace flowspace-local --from-file=tls.crt=.secrets/identity-session/workspace.crt --from-file=tls.key=.secrets/identity-session/workspace.key --from-file=ca.crt=.secrets/identity-session/ca.crt --dry-run=client -o json | kubeseal --cert .secrets/identity-session/sealing.crt --format yaml > deploy/overlays/local/workspace/session-tls-sealed-secret.yaml
kubeseal --controller-name sealed-secrets-controller --controller-namespace kube-system --validate < deploy/overlays/local/identity/session-tls-sealed-secret.yaml
kubeseal --controller-name sealed-secrets-controller --controller-namespace kube-system --validate < deploy/overlays/local/workspace/session-tls-sealed-secret.yaml
tilt up
task secrets
```

Tilt builds the API images and waits for the Sealed Secrets controller before it applies either sealed Secret. The Identity API loads `/keys/session/tls.crt`, `/keys/session/tls.key`, and `/keys/session/ca.crt`. Workspace receives the same paths from its own Secret. Run `sh scripts/smoke-identity-session-local.sh` in another terminal after the Identity rollout to check the approved and rejected certificate paths. Apply updates through Tilt; applying the whole Kustomize overlay to a running cluster can fail on an already completed migration Job because Kubernetes does not permit changes to its pod template.

## Replace certificates

Before a certificate expires, generate a new leaf key and certificate using the commands above with a `-next` filename. Keep the same CA unless the CA itself is being replaced. Use the saved `identity.ext` or `workspace.ext` file when you sign the replacement. In the matching sealing command, point `tls.crt` and `tls.key` at the `-next` files. For an Identity server certificate, update `identity-session-tls` through Tilt and restart `identity-api`. Workspace continues to trust the same CA and Service DNS name.

For a Workspace client certificate, add the new public-key fingerprint to `SESSION_CALLER_ALLOWLIST` beside the old fingerprint. Apply the Identity overlay through Tilt and restart `identity-api` before you replace the Workspace Secret and restart `workspace-api`. Move the `workspace-next` files to the current local filenames and run the smoke test. Then remove the old fingerprint and restart Identity and Workspace to close old TLS connections. If the old key leaked, remove its fingerprint immediately and restart both workloads without an overlap period. Never reuse a leaf private key for a replacement certificate.

If the CA changes, issue both leaf certificates from the new CA, update both Sealed Secrets, and roll out both workloads together. If the local cluster is rebuilt, restore the Sealed Secrets controller keys before applying these manifests, or create new certificates and reseal both Secrets for the new controller key. [Sealed Secrets documents strict scope and key renewal](https://github.com/bitnami/sealed-secrets#scopes).
