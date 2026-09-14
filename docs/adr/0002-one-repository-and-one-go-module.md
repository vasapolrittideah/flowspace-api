# ADR-0002: One repository and one root Go module

## Status

Accepted

## Date

2026-09-14

## Context

One developer and AI agents maintain related services, contracts, deployment definitions, and documentation. The services require independent binaries and deployments, but they do not yet require independent dependency versions, ownership policies, or release repositories.

## Decision

Keep FlowSpace in one repository with one `go.mod` at the root. Each application service has its own binaries, container image, database assets, and private packages while repository-wide generation and checks run from the root. Do not add per-service modules or a `go.work` file until independent dependency management becomes a concrete need.

## Alternatives Considered

### One repository with a Go module per service

- Pros: services can upgrade dependencies independently while code remains together.
- Cons: local replacements, workspace configuration, and repeated dependency maintenance add coordination without a current consumer.
- Rejected: one dependency graph is enough for the current team and release model.

### One repository per service

- Pros: governance, history, and releases are fully independent.
- Cons: shared contract and deployment changes require coordinated repositories and pull requests.
- Rejected: the operational separation does not yet justify repository separation.

## Consequences

- Services share one Go dependency graph while retaining separate builds and releases.
- Contract, service, and deployment changes can be reviewed together.
- Root tooling can run repository-wide checks without a workspace layer.
- Splitting modules later will be an explicit new decision.
