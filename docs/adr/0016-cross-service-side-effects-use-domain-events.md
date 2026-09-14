# ADR-0016: Cross-service side effects use domain events

## Status

Accepted

## Date

2026-09-14

## Context

Workspace membership changes must reach authorization projections, and committed Work changes must create notifications. Making those side effects synchronous would make the receiving service's availability and latency part of the originating transaction.

## Decision

Integrate cross-service side effects through asynchronous, versioned domain events. Workspace publishes membership and role changes for local authorization projections. Work publishes task events for Notifications. A durable broker retains events and consumer offsets, while event contracts use registered schemas and stable event identifiers.

## Alternatives Considered

### Synchronous service calls for every side effect

- Pros: the caller learns immediately whether the receiver handled the request.
- Cons: receiver outages and latency become failures of otherwise valid local mutations.
- Rejected: authorization replication and notifications may lag without invalidating the source commit.

### Change data capture from database logs

- Pros: application writes need no explicit event publication path.
- Cons: database row changes do not state domain intent and add a second infrastructure integration.
- Rejected: producers should name the facts consumers are allowed to depend on.

## Consequences

- A successful mutation means the source state committed, not that every consumer caught up.
- Consumer lag, replay, poison records, and schema retention are normal operating concerns.
- Cross-service views can be temporarily stale.
- Event availability does not replace synchronous contracts where an immediate answer is required.
