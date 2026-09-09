# ADR-007: Runtime environments

## Status

Accepted.

## Date

2026-09-09 (consolidated).

## Context

Managed application hosting is outside the budget, and operating Kubernetes is part of the learning goal. The available server is one Ubuntu host with local SSD storage; no independent backup destination exists yet.

## Decision

Run local development in a lightweight Kubernetes cluster inside Docker Desktop on Mac. Run staging and the environment named production on self-hosted Kubernetes on Ubuntu. Use node-local persistent volumes initially and treat all data as disposable learning data.

Before onboarding real teams, add independent backup storage and prove restoration from it. Local copies or replicas on the same host do not satisfy host-loss recovery.

The selected Kubernetes distributions, local control loop, and volume provisioner are listed in [technology choices](../technology-choices.md).

## Alternatives Considered

Managed hosting does not fit the stated constraint. Distributed storage on one physical host adds machinery without surviving host loss. Requiring new backup hardware now would block the agreed learning stage.

## Consequences

Neither namespaces nor multiple replicas on this host create independent failure domains. Resource fit, storage retention, reclaim policies, backup targets, and cluster topology must be measured or accepted separately. “Production” is an environment name, not a readiness claim.
