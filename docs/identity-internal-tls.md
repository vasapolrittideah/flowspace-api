# Local Identity session TLS

Identity serves `CheckSession` at the cluster-only `identity-session.flowspace-local.svc:8082` Service. The local NetworkPolicy admits Workspace pods to this port. Identity requires TLS 1.3 and a Workspace client certificate signed by the local CA, with URI `urn:flowspace:service:workspace` and an approved public-key fingerprint. Workspace must validate Identity's server certificate against the same CA and the Service DNS name. Identity's public API remains on port 8080, while health and signing keys are on the separate `identity-internal` Service at port 8081. Workspace will call `CheckSession` in Issue #116.

## Set up local certificates

With the `k3d-flowspace` context selected and `helm`, `kubeseal`, `openssl`, `kubectl`, Tilt, Task, and `grpcurl` installed, run this from the repository root:

```sh
task identity:session-tls:setup
tilt up
```

After Tilt deploys Identity, run these checks in another terminal:

```sh
task secrets
sh scripts/smoke-identity-session-local.sh
```

The setup script installs the pinned Sealed Secrets controller, creates one local CA and three leaf certificates, seals separate Identity and Workspace Secrets, validates both manifests, and sets the Workspace certificate fingerprint in Identity's allowlist. It stops if any private key already exists. Keep `.secrets/identity-session/` outside Git. Back up the CA key and the controller keys outside Git before relying on the encrypted manifests after a cluster rebuild. The CA private key must never enter Kubernetes.

Tilt builds the API images and waits for the controller before it applies either SealedSecret. Identity and Workspace load their own `tls.crt`, `tls.key`, and the public `ca.crt` from `/keys/session/`. Apply updates through Tilt; applying the whole Kustomize overlay to a running cluster can fail on an already completed migration Job because Kubernetes does not permit changes to its pod template.

## Replace certificates

Before a leaf certificate expires, generate a new key and certificate with `-next` filenames, using the same CA and the matching `.ext` file saved by the setup script. Re-seal the matching Secret with those files, validate it with `kubeseal --validate`, and apply it through Tilt. For an Identity server certificate, restart `identity-api`; Workspace continues to trust the same CA and Service DNS name.

For a Workspace client certificate, add the new public-key fingerprint to `SESSION_CALLER_ALLOWLIST` beside the old fingerprint. Apply the Identity overlay through Tilt and restart `identity-api` before replacing the Workspace Secret and restarting `workspace-api`. Move the `workspace-next` files to the current local filenames and run the smoke test. Then remove the old fingerprint and restart both workloads to close old TLS connections. If the old key leaked, remove its fingerprint immediately and restart both workloads without an overlap period. Never reuse a leaf private key for a replacement certificate.

If the CA changes, issue both leaf certificates from the new CA, update both Sealed Secrets, and roll out both workloads together. If the local cluster is rebuilt, restore the Sealed Secrets controller keys before applying these manifests, or create new certificates and reseal both Secrets for the new controller key. [Sealed Secrets documents strict scope and key renewal](https://github.com/bitnami/sealed-secrets#scopes).
