# ADR-0031: FlowSpace owns authentication and revocable sessions

## Status

Proposed

## Date

2026-09-23

## Context

[ADR-0019](0019-keycloak-owns-authentication-flows.md) assigns authentication to Keycloak. [ADR-0001](0001-one-bounded-context-per-service.md) treats Keycloak as a supporting component rather than a FlowSpace service. The project now aims to learn how to build and operate authentication for FlowSpace itself.

The first release uses API clients and disposable data. Later releases may serve real users, but only after the project meets its recovery and security requirements. FlowSpace needs email and password login, Google and GitHub login, email verification, password reset, and logout from one or all devices. Logout must stop new requests from those devices as soon as it succeeds. No external application needs to use FlowSpace as an identity provider.

The current Workspace adapter validates a Keycloak token without checking session state. Signature and expiry checks alone cannot enforce immediate logout. Workspace must continue to own memberships, roles, and authorization.

## Decision

Build one independently deployable Go Identity service in the existing repository and Go module. Identity owns accounts, passwords, provider links, verified email addresses, recovery challenges, and sessions in its own PostgreSQL database. Each account has a stable subject that does not change with its email address or provider account.

Identity issues opaque bearer session tokens with cryptographically secure random values and stores only their hashes. Each protected service asks Identity to validate the session before admitting every request. A successful validation returns the subject and verified email status, not a workspace role. The service does not cache a successful validation, because a cache would delay revocation. Validation fails closed when Identity cannot confirm session state.

Identity revokes a selected session or all sessions in its database before reporting logout success. Validation calls that start after that commit reject revoked sessions. Calls already in progress may finish.

An account with an unverified email address can complete verification and request a new code but cannot use Workspace. Identity accepts a provider's verified email only after validating the provider response. A signed-in user must explicitly link a Google or GitHub account; matching email addresses never link accounts automatically.

## Alternatives Considered

### Keep Keycloak

- Pros: Keycloak already owns credential storage, recovery, and provider integration.
- Cons: FlowSpace would not implement the authentication behavior that this learning goal targets.
- Rejected: The project now explicitly includes building and operating its own authentication service.

### Issue signed tokens with a revocation lookup

- Pros: Existing token validation and OIDC discovery could remain closer to their current form.
- Cons: Immediate logout still requires a session lookup on every request, plus signing keys and token metadata.
- Rejected: Opaque tokens meet the FlowSpace-only need with fewer moving parts.

### Accept logout after a short token lifetime

- Pros: Protected services could validate signed tokens without contacting Identity.
- Cons: A revoked device could keep making requests until its token expires.
- Rejected: Logout must stop new requests immediately.

## Consequences

- Identity becomes a fourth FlowSpace service and a dependency for each protected request. An Identity or database outage denies new protected requests.
- Protected services must replace Keycloak token validation with authenticated internal session validation. They must still authorize workspace access from Workspace-owned data.
- On acceptance, this record supersedes ADR-0019 and the Keycloak and three-service parts of ADR-0001. The architecture and technology stack must then reflect the new boundary.
- API contracts, a threat model, session lifetime rules, six-digit code limits, provider failure cases, and tests must be specified before implementation.
- The initial release uses Mailpit and disposable data. Real email delivery, MFA, independent backups, and tested restoration are required before serving real users.
