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

Create a short-lived branch and start the local cluster before replacing any certificate. Renew a leaf certificate before it expires with one of these tasks:

```sh
task identity:session-tls:rotate-server
task identity:session-tls:rotate-client
```

Run only the task for the certificate you need to replace.

- Each task creates a new private key and certificate from the existing CA, verifies and seals the certificate, applies only the matching SealedSecret, waits for the Secret to update, restarts the workload, and runs the smoke test.
- The client task adds the new fingerprint to Identity's allowlist, tests the new certificate, then removes the old fingerprint and restarts both workloads to close existing connections.
- Review and commit the changed encrypted manifest and, for a client rotation, the allowlist. Old keys and manifests remain in the ignored `.secrets/identity-session/rotation.*` directory printed by the task for rollback.

If a task stops, inspect the live Secret, allowlist, and saved files before retrying. If the Workspace client key leaked, run `task identity:session-tls:revoke-client`; it removes the old fingerprint before switching the Secret and briefly interrupts session checks. Never reuse a leaf private key. The setup task creates a new CA and is not a renewal command.

To replace the CA and all three leaf certificates together, run `task identity:session-tls:rotate-ca`. The task validates both new SealedSecrets before applying them, restarts Identity and Workspace, and tests the new CA and certificates. Session checks can briefly fail while the two workloads restart. Review and commit both encrypted manifests and the updated allowlist. The prior CA key and certificates remain in the ignored archive printed by the task.

If the local cluster is rebuilt, restore the saved controller keys before `tilt up` to reuse the committed manifests. [Sealed Secrets documents key backup and restore](https://github.com/bitnami/sealed-secrets#how-can-i-do-a-backup-of-my-sealedsecrets).

## Rebuild with a new controller key

Use this procedure after deleting the local cluster. Keep the CA and both service certificates and private keys in `.secrets/identity-session/`. Run the commands from the repository root on a short-lived branch before `tilt up`. The reseal task checks the certificates and Workspace allowlist. It installs the pinned controller and validates both encrypted manifests before replacing them. The setup task refuses existing private keys, and the rotation tasks require live Secrets.

```sh
task cluster:create
kubectl config use-context k3d-flowspace
task identity:session-tls:reseal
tilt up
```

After Tilt starts, make sure that both SealedSecrets show `True` in another terminal:

```sh
kubectl --context k3d-flowspace -n flowspace-local get sealedsecret identity-session-tls workspace-session-tls -o 'custom-columns=NAME:.metadata.name,SYNCED:.status.conditions[0].status'
sh scripts/smoke-identity-session-local.sh
```

The Workspace allowlist stays the same because this procedure reuses its certificate.

Review and commit both encrypted manifests, then open a PR.
