# ADR-0030: Telemetry is bounded and non-blocking

## Status

Accepted

## Date

2026-09-14

## Context

The observability stack shares one constrained host with application workloads. Unbounded retention, cardinality, buffering, or tracing can exhaust that host, while a failed collector must not turn diagnostic loss into an application outage.

## Decision

Telemetry failures do not block application requests. Bound retention, trace sampling, export buffering, and metric cardinality. Measure ingestion and resource use before increasing fidelity or retention, and prefer dropping telemetry within an explicit bound over exhausting application resources.

## Alternatives Considered

### Fail requests when telemetry export fails

- Pros: every successful request is guaranteed to have exported diagnostic evidence.
- Cons: an observability outage becomes an application outage.
- Rejected: telemetry observes application behavior and must not control its availability.

### Retain every signal at full fidelity

- Pros: maximum forensic detail for every request and event.
- Cons: storage and cardinality grow without a limit on one host.
- Rejected: the learning environment needs predictable resource use more than unlimited history.

## Consequences

- Some telemetry may be sampled or dropped during pressure or collector failure.
- Capacity, retention, sampling, and cardinality limits require monitoring.
- Application success does not imply successful telemetry export.
- Increasing observability fidelity requires measured resource headroom.
