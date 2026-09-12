# ADR-001: Service architecture

## Status

Accepted.

## Date

2026-09-09.

## Context

FlowSpace is an educational project for learning distributed-system behavior. Its first capabilities are workspace membership, project and task collaboration, and notifications. One developer maintains the system on constrained self-hosted infrastructure.

## Decision

Implement independently deployable Go services, introduced incrementally:

- Workspace owns workspaces, memberships, invitations, roles, and authorization decisions.
- Work owns projects, tasks, assignments, comments, and local activity.
- Notifications owns the application inbox and event-processing records.
- Keycloak remains a supporting identity component, not a custom FlowSpace service.

Keep projects, tasks, assignments, and comments together so their invariants can share local transactions. Keep Notifications separate because delayed inbox delivery must not determine whether a task mutation succeeds.

Within a service, keep domain and application rules independent of transports and storage. Add ports and adapters only when real inbound or outbound integrations require them; wire dependencies with ordinary constructors.

## Alternatives Considered

A modular monolith would reduce operational cost but would not meet the explicit distributed-systems learning goal. A service per entity would split related invariants before independent ownership or scaling requires it. A custom authentication service would add credential and federation responsibilities outside the initial domain.

## Consequences

Network failure, contract evolution, and cross-service consistency are intentional learning costs. Service boundaries may change when ownership, workload, or consistency evidence justifies it. Microservices are not a demonstrated capacity requirement, and empty service skeletons are not created ahead of implementation.
