# ADR-0022: Single-host storage holds disposable learning data

## Status

Accepted

## Date

2026-09-14

## Context

The self-hosted cluster has one Ubuntu host with local SSD storage and no independent backup destination. Replicas, namespaces, or distributed storage processes on that host cannot preserve data when the host itself is lost.

## Decision

Use node-local persistent volumes initially and treat all stored data as disposable learning data. Before onboarding real teams, add backup storage independent of the host and prove restoration from it. A local copy or another replica on the same physical host does not satisfy that gate.

## Alternatives Considered

### Distributed storage on the same host

- Pros: exercises replication and storage orchestration.
- Cons: every replica still disappears with the one physical failure domain.
- Rejected: machinery without host-loss recovery does not protect the data.

### Require independent backup hardware immediately

- Pros: durable user data can be supported from the first deployment.
- Cons: it blocks the agreed disposable learning stage on new infrastructure.
- Rejected: the readiness gate prevents real use until the missing protection exists.

## Consequences

- Host loss may destroy all current application data.
- Storage retention and reclaim behavior still require explicit configuration.
- Backups are not considered complete until a restore is demonstrated.
- Real teams cannot be onboarded while this record remains the active storage decision.
