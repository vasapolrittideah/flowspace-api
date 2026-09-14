# ADR-0020: Services authorize from bounded local projections

## Status

Accepted

## Date

2026-09-14

## Context

Workspace owns memberships and roles, but making every protected operation call Workspace would add a synchronous availability dependency. An unbounded local cache would preserve availability by allowing access indefinitely after revocation.

## Decision

Workspace owns invitations, memberships, roles, and application authorization. It publishes ordered, versioned authorization changes plus periodic watermarks. Work and Notifications maintain local projections and authorize only while the latest applied watermark is within a configured maximum staleness bound and the consumer is healthy.

Fail closed with a temporary service error when no valid projection exists, an event-version gap is detected, or the staleness bound is exceeded. Evaluate authorization when admitting a request; an admitted operation may finish during concurrent revocation, but subsequent requests are denied after the newer version is applied. Never fall back to client-supplied claims.

## Alternatives Considered

### Call Workspace for every protected request

- Pros: authorization always uses the owner's latest committed state.
- Cons: Workspace latency or outage blocks every protected operation in other services.
- Rejected: a bounded local projection makes the availability and revocation trade-off explicit.

### Cache authorization without a freshness bound

- Pros: services continue authorizing through arbitrarily long Workspace or broker outages.
- Cons: revoked access can remain valid indefinitely.
- Rejected: security-sensitive stale state must fail closed.

## Consequences

- Authorization-event lag is security-relevant and monitored against the configured bound.
- Projection updates, event identity, version, and checkpoint commit in one local transaction before the broker offset.
- Brief Workspace outages need not block requests while a projection remains fresh.
- Cross-service revocation has an explicit bounded delay rather than instantaneous semantics.
