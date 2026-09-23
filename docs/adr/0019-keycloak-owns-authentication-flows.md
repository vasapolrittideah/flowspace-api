# ADR-0019: Keycloak owns authentication flows

## Status

Superseded by [ADR-0031](0031-flowspace-owns-authentication-and-revocable-sessions.md)

## Date

2026-09-14

## Context

FlowSpace needs local login, social login, recovery, and room for later federation without owning password storage or identity-provider protocol details. Email addresses can change, so established membership cannot use email as the durable identity key.

## Decision

Delegate identity and authentication flows to self-hosted Keycloak. Begin with local email and password plus Google login; defer LinkedIn and enterprise federation until required. Use the stable token subject for established identity links. Keep future branded authentication pages as Keycloak-hosted themes so Keycloak continues to own authentication state and browser flows.

## Alternatives Considered

### Build a FlowSpace authentication service

- Pros: complete control over screens, credentials, and recovery behavior.
- Cons: the project would own password security, account recovery, provider linking, and federation protocols.
- Rejected: those responsibilities are outside the initial application domain.

### Use email as the identity key

- Pros: human-readable membership records and simple lookup.
- Cons: address changes or provider differences can split or transfer identity incorrectly.
- Rejected: the provider subject is the stable authenticated identifier.

## Consequences

- Keycloak state, upgrades, themes, provider linking, email trust, and token validation require operation and testing.
- FlowSpace stores stable subjects for established memberships.
- Adding an identity provider is a Keycloak integration rather than a new application credential flow.
- Authentication alone grants no workspace access.
