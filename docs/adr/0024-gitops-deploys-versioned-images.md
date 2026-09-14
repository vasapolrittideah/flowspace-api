# ADR-0024: GitOps deploys versioned images

## Status

Accepted

## Date

2026-09-14

## Context

Deployments need reviewable desired state and reproducible service images. Allowing CI to push directly into the cluster would place cluster credentials in the CI trust boundary and make deployed state harder to reconstruct from Git.

## Decision

Publish public, versioned service images to a registry and record the desired versions in Git-managed deployment state. A reconciler inside the destination cluster pulls and applies that state. CI builds and publishes artifacts but receives no direct cluster deployment credentials.

## Alternatives Considered

### CI pushes deployments to the cluster

- Pros: one workflow builds and immediately deploys the artifact.
- Cons: CI requires cluster credentials and deployment state can diverge from the repository.
- Rejected: the destination cluster should pull reviewed desired state.

### Deploy mutable image tags

- Pros: manifests rarely need version updates.
- Cons: the same desired state can resolve to different binaries over time.
- Rejected: a deployment revision must identify an immutable artifact version.

## Consequences

- Publishing and deployment credentials remain separately scoped.
- Public images contain no secrets.
- Deployment changes are reviewable as Git changes.
- Migration ordering, promotion, and rollout verification still require accepted procedures.
