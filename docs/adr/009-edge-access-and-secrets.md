# ADR-009: Edge access and secrets

## Status

Accepted.

## Date

2026-09-09 (consolidated).

## Context

Selected self-hosted application routes must be reachable without exposing infrastructure administration. Staging needs an additional admission gate, and Git-managed deployment must not put plaintext credentials in the public repository.

## Decision

Use an outbound managed tunnel for selected application routes. Protect staging's public hostname with an edge access policy while retaining normal FlowSpace authentication and authorization. Expose production's user-facing API publicly over HTTPS, but keep cluster, database, broker, deployment, and identity administration private.

Commit only encrypted Kubernetes secret manifests. Decrypt them inside the destination cluster, keep plaintext and decryption keys outside Git, scope secrets to their environment and workload, and use distinct environment credentials.

The selected edge and secret-management tools are listed in [technology choices](../technology-choices.md).

## Alternatives Considered

Direct inbound exposure creates a different network path. LAN-only staging would not meet the selected public-hostname workflow. Manually created runtime secrets leave deployment state outside Git; an external secret store adds another service before cross-system lifecycle management is needed.

## Consequences

The edge provider is an external availability dependency and does not replace application authorization. Secret-controller keys become recovery dependencies. Exact hostnames, allowlists, administrative access, origin TLS, proxy headers, bootstrap, and rotation procedures remain implementation proposals.
