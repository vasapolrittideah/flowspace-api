# ADR-0001: One bounded context per independently deployable service

## Status

Accepted, except the service count and Keycloak boundary superseded by [ADR-0031](0031-flowspace-owns-authentication-and-revocable-sessions.md)

## Date

2026-09-14

## Context

FlowSpace is an educational distributed-systems project whose first capabilities fall into three vocabularies: workspace membership and authorization, collaborative work, and notifications. Related invariants need local transactions, while delayed notification delivery must not determine whether a task mutation succeeds. The project should expose real network and consistency behavior without creating a service for every entity.

## Decision

Use three independently deployable application services, introduced only when their capabilities are implemented:

- Workspace owns workspaces, memberships, invitations, roles, and authorization decisions.
- Work owns projects, tasks, assignments, comments, and local activity.
- Notifications owns the application inbox and event-processing records.

Keycloak remains a supporting identity component rather than a fourth FlowSpace service. A new service requires evidence of separate vocabulary, ownership, release cadence, or scaling needs.

## Alternatives Considered

### A modular monolith

- Pros: fewer deployments, no cross-service network failures, and simpler transactions.
- Cons: it removes much of the distributed-system behavior the project exists to exercise.
- Rejected: the learning goal explicitly requires independently deployable boundaries.

### A service per entity

- Pros: every resource can deploy and scale independently.
- Cons: invariants among projects, tasks, assignments, and comments become distributed transactions without a separate ownership need.
- Rejected: splitting related concepts creates coordination cost before evidence justifies it.

## Consequences

- Network failure, contract evolution, and cross-service consistency are intentional system concerns.
- Related Work invariants remain inside one transaction boundary.
- Notification outages may delay inbox delivery without failing committed Work mutations.
- Empty service skeletons are not created in advance.
