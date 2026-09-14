# ADR-0026: An outbound tunnel exposes selected routes

## Status

Accepted

## Date

2026-09-14

## Context

Selected application routes must be reachable from outside the self-hosted network without publishing infrastructure administration. Staging also needs an admission gate before normal FlowSpace authentication, while the production user-facing API needs public HTTPS access.

## Decision

Use an outbound managed tunnel for selected application routes. Protect the staging hostname with an edge access policy in addition to normal application authentication and authorization. Expose the production user-facing API publicly over HTTPS, but keep cluster, database, broker, deployment, and identity administration private.

## Alternatives Considered

### Direct inbound exposure

- Pros: no managed tunnel dependency and a conventional public listener.
- Cons: it opens and secures a different inbound network path to the host.
- Rejected: an outbound tunnel publishes only the selected routes without general inbound access.

### Keep staging available only on the local network

- Pros: no public staging hostname or edge policy.
- Cons: it does not support the selected remote public-hostname workflow.
- Rejected: staging needs controlled remote access before application login.

## Consequences

- The edge provider becomes an external availability dependency.
- Edge admission never replaces application authentication or authorization.
- Administrative services remain unreachable through public application routes.
- Exact hostnames, origin TLS, proxy headers, and allowlists remain implementation work.
