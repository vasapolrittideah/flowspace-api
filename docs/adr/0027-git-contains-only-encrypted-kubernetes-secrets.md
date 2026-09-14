# ADR-0027: Git contains only encrypted Kubernetes secrets

## Status

Accepted

## Date

2026-09-14

## Context

Git-managed deployment needs reviewable secret resources without placing plaintext credentials in a public repository. Manually creating all runtime secrets would leave part of the desired deployment state outside version control.

## Decision

Commit only encrypted Kubernetes secret manifests and decrypt them inside the destination cluster. Keep plaintext values and decryption keys outside Git. Scope secrets to their environment and workload and use distinct credentials across environments.

## Alternatives Considered

### Manually create runtime secrets

- Pros: no encrypted secret controller or key recovery process.
- Cons: deployed secret names and structure are not fully represented in Git.
- Rejected: encrypted manifests keep desired resources reviewable without exposing values.

### Add an external secret store immediately

- Pros: centralized rotation and cross-system secret lifecycle management.
- Cons: it adds another service before that lifecycle is required.
- Rejected: in-cluster decryption covers the current GitOps need with fewer components.

### Render plaintext Secrets from values

- Pros: simple deployment templates and no decryption controller.
- Cons: credentials enter source, CI output, or release values.
- Rejected: plaintext secrets must never enter Git or deployment logs.

## Consequences

- Secret-controller keys become recovery dependencies.
- Encrypted manifests can be reviewed and reconciled from Git.
- Bootstrap and rotation procedures must protect plaintext outside the repository.
- A credential leak in one environment does not intentionally grant access to another.
