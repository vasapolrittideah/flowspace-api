# ADR-006: Identity and authorization

## Status

Accepted.

## Date

2026-09-10.

## Context

One login identity may belong to several workspaces with a different role in each. FlowSpace needs local and social login now, recovery flows, and room for later federation without owning credential handling.

## Decision

Delegate identity and authentication flows to self-hosted Keycloak. Start with local email/password and Google login; defer LinkedIn and enterprise SSO. Use stable identity subjects rather than email for established memberships.

Workspace owns memberships, workspace roles, invitations, and application authorization. Authentication alone grants no workspace access. Workspace publishes ordered, versioned membership and role changes plus periodic authorization watermarks; Work and Notifications maintain local authorization projections from those events.

Authorize requests from the local projection only while its freshness proof—the latest applied Workspace watermark—is within a configured maximum staleness bound and the consumer is healthy. If no valid projection exists, an event-version gap is detected, or the bound is exceeded, fail closed with a temporary service error. Do not fall back to claims supplied by the client.

Evaluate authorization when admitting a request. An admitted operation may finish during concurrent revocation; after a service applies the newer authorization version, subsequent requests are denied. Ownership and membership changes remain atomic inside Workspace.

Future branded authentication pages remain Keycloak-hosted themes so Keycloak keeps control of authentication state and browser flows. Test-email tooling is listed in [technology choices](../technology-choices.md).

## Alternatives Considered

A custom authentication service would add credential, recovery, and federation risk. Global roles in the identity provider would conflate identity with per-workspace authorization. Synchronous Workspace checks would make Workspace availability part of every protected request. An unbounded cache would allow indefinite stale access. Rechecking before commit would not remove distributed revocation races. Separate password forms would bypass the selected browser-based identity flows.

## Consequences

Keycloak state, upgrades, themes, provider linking, email trust, and token validation require explicit operation and testing. FlowSpace services must validate tokens and enforce workspace permissions independently of edge access controls.

Authorization-event lag becomes security-relevant and must be monitored against the configured bound. Projection rebuilds must preserve event order and reject older versions. This improves availability during brief Workspace outages but creates an explicit bounded revocation delay; it does not provide instantaneous cross-service revocation.

Projection consumers apply the authorization change, version, event ID, and checkpoint in one local database transaction before committing the broker offset. Redelivery is therefore idempotent and cannot advance the checkpoint past unapplied authorization state.
