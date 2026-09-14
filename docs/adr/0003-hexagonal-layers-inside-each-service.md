# ADR-0003: Hexagonal layers inside each service

## Status

Accepted

## Date

2026-09-14

## Context

Business rules should outlive transports, databases, brokers, and identity providers. Without an enforced dependency direction, generated messages, SQL rows, and infrastructure clients can leak into the domain and make those rules difficult to test or replace.

## Decision

Organize each service as a hexagonal architecture. Domain code contains business invariants and depends on no transport or infrastructure. Application code coordinates domain behavior through inbound and outbound ports. Adapters validate and map external protocols or implement infrastructure capabilities. Bootstrap is the composition root and wires concrete implementations with ordinary constructors.

Add a port only when application code has a real boundary to call. Keep layers flat until concrete code requires another package.

## Alternatives Considered

### Packages grouped only by technical type

- Pros: handlers, models, and repositories are familiar places for new files.
- Cons: one capability is spread across technical packages and the allowed dependency direction is unclear.
- Rejected: the domain would have no enforceable boundary from infrastructure.

### Direct dependencies on concrete infrastructure

- Pros: fewer interfaces and mappings for the first implementation.
- Cons: business behavior becomes coupled to drivers and requires infrastructure-heavy tests.
- Rejected: the learning project needs visible boundaries without speculative abstractions.

## Consequences

- Domain tests need no transport or infrastructure.
- Use-case tests replace only real outbound boundaries.
- Adapters perform protocol mapping without owning business rules.
- Each real integration adds a port and mapping cost at the service edge.
