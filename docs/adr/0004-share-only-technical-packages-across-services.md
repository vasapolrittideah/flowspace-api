# ADR-0004: Only technical packages are shared across services

## Status

Accepted

## Date

2026-09-14

## Context

A monorepo makes it easy for one service to import another service's types or to place unrelated helpers in a generic shared package. That convenience would couple domain ownership and persistence implementations even though the services are intended to evolve and deploy independently.

## Decision

Reserve root `internal/` for narrowly named technical packages with multiple concrete service consumers. Keep domain models, repositories, use cases, and service-specific helpers under their owning service. One service never imports another service's private packages. Cross-process data is exchanged through versioned contracts rather than shared domain types.

Do not create generic `common`, `helpers`, `models`, `repositories`, or `utils` packages. Keep code beside its only consumer until reuse is real.

## Alternatives Considered

### Share domain models across services

- Pros: fewer duplicate types and mappings.
- Cons: an internal model change becomes a coordinated release and obscures which service owns its invariants.
- Rejected: cross-service contracts, not shared implementation types, define integration.

### Put reusable-looking code in generic packages immediately

- Pros: future callers can find helpers in one place.
- Cons: speculative reuse creates weak ownership and unrelated dependencies.
- Rejected: promotion is cheap after a second concrete consumer appears.

## Consequences

- Similar service-owned code may be duplicated until a stable technical boundary emerges.
- Shared packages have explicit technical purposes and multiple consumers.
- Services cannot bypass contracts by importing another service's implementation.
- Moving code to root `internal/` requires evidence of actual reuse.
