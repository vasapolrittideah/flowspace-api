# ADR-0021: Kubernetes runs locally and on the self-hosted server

## Status

Accepted

## Date

2026-09-14

## Context

Operating Kubernetes is part of the project's learning goal, managed application hosting is outside the budget, and the available server is one Ubuntu host. Local development still needs a reproducible environment close enough to exercise the same deployment objects.

## Decision

Run local development in a lightweight Kubernetes cluster inside Docker Desktop on Mac. Run staging and the environment named production on self-hosted Kubernetes on Ubuntu. Keep the selected distributions and local control loop in the [technology stack](../technology-stack.md) so they can change without altering the runtime contract.

## Alternatives Considered

### Managed application hosting

- Pros: less cluster operation and built-in infrastructure services.
- Cons: it exceeds the zero-spend constraint and removes the selected Kubernetes operations exercise.
- Rejected: cost and learning goals both favor self-hosting.

### Run services directly as local processes

- Pros: faster startup and fewer local dependencies.
- Cons: deployment objects, service discovery, and cluster behavior are not exercised until a later environment.
- Rejected: local development should validate the deployment model used elsewhere.

## Consequences

- Developers operate a local cluster and Docker runtime.
- Staging and production-named workloads share one self-hosted physical server.
- Cluster distribution changes remain tool replacements if deployment behavior is preserved.
- The name production is not a production-readiness claim.
