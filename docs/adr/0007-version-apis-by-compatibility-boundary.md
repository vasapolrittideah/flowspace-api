# ADR-0007: APIs are versioned by compatibility boundary

## Status

Accepted

## Date

2026-09-14

## Context

Public REST callers and internal RPC callers share generated contracts. Consumers need to know which changes are additive and which require coordinated migration rather than discovering an incompatibility at runtime.

## Decision

Begin Protobuf packages at `flowspace.<service>.v1` and expose annotated public routes below `/v1` using plural resource nouns. Evolve a version additively. An incompatible change requires a new Protobuf package and REST route version while the previous version remains available for its supported migration period.

## Alternatives Considered

### One unversioned API

- Pros: shorter package names and routes.
- Cons: an incompatible change has no explicit migration boundary.
- Rejected: implicit compatibility is unsafe for independently deployed consumers.

### Version every individual endpoint

- Pros: only a changed method receives a new version.
- Cons: one service surface accumulates mixed versions and inconsistent shared message types.
- Rejected: the package is the compatibility unit used by generation and review.

## Consequences

- Additive fields and methods stay within the current version.
- Breaking changes require parallel contracts and an explicit migration.
- REST and RPC versions advance together for the same synchronous contract.
- Compatibility checks can compare the package boundary mechanically.
