# ADR-010: Observability

## Status

Accepted.

## Date

2026-09-09.

## Context

Learning distributed failure behavior requires correlated requests, events, logs, metrics, and traces. Telemetry must remain bounded on the single host.

## Decision

Instrument services with vendor-neutral telemetry and propagate request and trace context across synchronous calls and events. Self-host collection, metrics, logs, traces, and dashboards, introduced incrementally and labeled by service and environment.

Telemetry failures must not block application requests. Bound retention, sampling, buffering, and metric cardinality. The selected instrumentation, collector, storage, and dashboard tools are listed in [technology choices](../technology-choices.md).

## Alternatives Considered

Logs alone make cross-service timing and aggregate behavior harder to investigate. Managed telemetry conflicts with the selected self-hosted learning stack. Backend-specific instrumentation would make later tool replacement more expensive.

## Consequences

The monitoring stack consumes memory and disk on the same host and shares its failure domain. Exact topology, retention, sampling, storage modes, alert routing, and capacity remain unresolved deployment work to validate with measured ingestion.
