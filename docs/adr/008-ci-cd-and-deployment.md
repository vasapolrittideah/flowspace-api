# ADR-008: CI/CD and deployment

## Status

Accepted.

## Date

2026-09-09.

## Context

The public learning repository needs reproducible checks, versioned images, and reviewable Kubernetes deployment state without maintaining another CI server.

## Decision

Host source and reviews in the public GitHub repository. Run hosted CI only within a strict zero-spend limit. Publish public, versioned service images to a registry and reconcile Kubernetes desired state from Git rather than granting CI direct cluster deployment access.

Use shared application manifests with explicit environment overlays and reuse maintained, pinned packages for third-party infrastructure. The selected CI, registry, GitOps, manifest, test, lint, load, and dependency-update tools are listed in [technology choices](../technology-choices.md).

## Alternatives Considered

A self-hosted runner would consume application-host resources. CI-driven cluster pushes require CI cluster credentials. Custom infrastructure packaging and duplicated environment manifests add maintenance without a current need.

## Consequences

Publishing credentials, deployment reconciliation, and environment credentials remain separately scoped. Public images contain no secrets. Exact release gates, migration ordering, image promotion, and deployment verification remain proposals in the [architecture overview](../architecture-overview.md).
