# ADR-0029: The observability stack is self-hosted

## Status

Accepted

## Date

2026-09-14

## Context

The project needs searchable logs, metrics, traces, and dashboards while remaining within its self-hosted, zero-spend learning constraints. A managed telemetry service would reduce operations but move storage and recurring cost outside those constraints.

## Decision

Self-host telemetry collection, metrics, logs, traces, and dashboards on the available infrastructure. Introduce each component incrementally when a service emits the signal it consumes. Keep the selected products in the [technology stack](../technology-stack.md).

## Alternatives Considered

### Managed observability

- Pros: less storage operation, upgrades, and capacity management.
- Cons: recurring cost and telemetry storage outside the selected self-hosted environment.
- Rejected: it conflicts with the project's operating constraints.

### Store telemetry only inside each workload

- Pros: no shared observability stack.
- Cons: restarts lose evidence and cross-service investigation requires visiting each process separately.
- Rejected: distributed behavior needs a correlated system view.

## Consequences

- The stack consumes memory, CPU, and disk on the constrained host.
- Telemetry shares a physical failure domain with the applications it observes.
- Components are added only when they have real signal producers and users.
- Product selection can change while the self-hosted and correlated behavior remains.
