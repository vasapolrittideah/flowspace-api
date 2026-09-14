# ADR-0025: Environments overlay shared manifests

## Status

Accepted

## Date

2026-09-14

## Context

Local, staging, and production-named environments deploy the same application services with different configuration and capacity. Copying complete manifests per environment makes common changes easy to miss, while writing custom packages for third-party infrastructure creates maintenance the upstream project already performs.

## Decision

Keep shared application manifests in one base and express environment differences through explicit overlays. Reuse maintained, pinned packages for third-party infrastructure rather than copying or rebuilding their deployment definitions. Add deployment files only when the corresponding workload exists.

## Alternatives Considered

### Complete manifests copied per environment

- Pros: every environment is self-contained and can diverge freely.
- Cons: common changes must be repeated and copies drift silently.
- Rejected: known differences should be visible as overlays on one application base.

### Custom manifests for all third-party infrastructure

- Pros: complete control and one manifest style throughout the repository.
- Cons: the project inherits upstream packaging, upgrade, and compatibility work.
- Rejected: maintained pinned packages are cheaper until a measured limitation requires ownership.

## Consequences

- Shared application changes land once in the base.
- Environment-specific differences remain reviewable in their overlays.
- Third-party upgrades are explicit package-version changes.
- Empty manifest trees are not scaffolded ahead of workloads.
