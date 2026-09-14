# ADR-0028: Telemetry is vendor-neutral and correlated

## Status

Accepted

## Date

2026-09-14

## Context

Learning distributed failure behavior requires following work across requests, synchronous calls, events, logs, metrics, and traces. Instrumentation tied directly to one storage backend makes that context expensive to preserve when the backend changes.

## Decision

Instrument services with vendor-neutral telemetry and propagate request and trace context across synchronous calls and events. Emit stable service and environment labels so logs, metrics, and traces can be correlated without backend-specific application code.

## Alternatives Considered

### Logs without metrics or traces

- Pros: one telemetry signal and simpler application instrumentation.
- Cons: cross-service timing and aggregate behavior are difficult to reconstruct.
- Rejected: the learning goals require correlated distributed evidence.

### Backend-specific instrumentation

- Pros: direct access to backend features and fewer translation layers.
- Cons: replacing storage or collection requires application changes.
- Rejected: the application should emit portable telemetry contracts.

## Consequences

- Context propagation is part of synchronous and asynchronous contracts.
- Services emit consistent identifying attributes.
- Collection and storage backends can change without rewriting business instrumentation.
- Instrumentation libraries and collectors remain replaceable technology choices.
