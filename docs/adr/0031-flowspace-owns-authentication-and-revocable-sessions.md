# ADR-0031: FlowSpace owns authentication and revocable sessions

## Status

Proposed

## Date

2026-09-23

## Context

[ADR-0019](0019-keycloak-owns-authentication-flows.md) assigns authentication to Keycloak. [ADR-0001](0001-one-bounded-context-per-service.md) treats Keycloak as a supporting component rather than a FlowSpace service. The project now aims to learn how to build and operate authentication for FlowSpace itself.

The first release uses API clients and disposable data. Later releases may serve real users, but only after the project meets its recovery and security requirements. FlowSpace needs email and password login, Google and GitHub login, email verification, password reset, and logout from one or all devices. Logout must stop new requests from those devices as soon as it succeeds. No external application needs to use FlowSpace as an identity provider.

The current Workspace adapter validates a Keycloak token without checking session state. The new design needs access and refresh tokens issued by FlowSpace. Signature and expiry checks alone cannot enforce immediate logout. Workspace must continue to own memberships, roles, and authorization.

## Decision

Build one independently deployable Go Identity service in the existing repository and Go module. Identity owns accounts, passwords, provider links, verified email addresses, recovery challenges, and sessions in its own PostgreSQL database. Each account has a stable subject that does not change with its email address or provider account.

Identity issues a short-lived JWT access token and an opaque refresh token for each device session. Identity signs access tokens with a private key and publishes public verification keys through a JSON Web Key Set (JWKS). Only Identity holds private signing keys. Each access token carries a stable subject, session ID, issuer, audience, issue time, expiry, and token ID. Protected services accept only the configured signing algorithm and verify the signature, token type, issuer, audience, and expiry.

API clients send access tokens in the `Authorization: Bearer` header on protected requests. Clients send refresh tokens only to Identity's refresh endpoint. Browser token storage remains a separate decision for the later web release.

Identity generates refresh tokens with cryptographically secure random values and stores only their hashes. A refresh request rotates the refresh token and issues a new access token. Reuse of an invalidated refresh token revokes the affected device session.

After local access-token verification, each protected service asks Identity whether the device session is still active before admitting every request. A successful check returns the current verified email status, not a workspace role. The service does not cache a successful session check, because a cache would delay revocation. The service denies the request when Identity cannot confirm session state.

Identity revokes a selected device session or all sessions and their refresh tokens before reporting logout success. Session checks that start after that commit reject access tokens from revoked sessions. Calls already in progress may finish.

Email verification and password reset use six-digit codes sent by email. An account with an unverified email address can complete verification and request a new code but cannot use Workspace. Identity accepts a provider's verified email only after validating the provider response. A signed-in user must explicitly link a Google or GitHub account; matching email addresses never link accounts automatically.

## Alternatives Considered

### Keep Keycloak

- Pros: Keycloak already owns credential storage, recovery, and provider integration.
- Cons: FlowSpace would not implement the authentication behavior that this learning goal targets.
- Rejected: The project now explicitly includes building and operating its own authentication service.

### Use one opaque session token

- Pros: One token and a session lookup need fewer moving parts.
- Cons: The design does not provide separate access and refresh tokens or use public-key verification.
- Rejected: FlowSpace will use separate tokens and an asymmetric signing key pair.

### Verify signed access tokens without a session check

- Pros: Protected services could validate access tokens without contacting Identity.
- Cons: A revoked device could keep making requests until its access token expires.
- Rejected: Logout must stop new requests immediately.

## Consequences

- Identity becomes a fourth FlowSpace service and a dependency for each protected request, even though services verify access-token signatures locally. An Identity or database outage denies new protected requests.
- Protected services must replace Keycloak token validation with FlowSpace access-token verification and authenticated internal session checks. They must still authorize workspace access from Workspace-owned data.
- Identity must protect and rotate private signing keys, publish matching public keys, and keep old public keys available while valid access tokens use them.
- On acceptance, this record supersedes ADR-0019 and the Keycloak and three-service parts of ADR-0001. The architecture and technology stack must then reflect the new boundary.
- API contracts, a threat model, signing algorithm and key rotation rules, token lifetimes, refresh retry rules, six-digit code limits, provider failure cases, and tests must be specified before implementation.
- The initial release uses Mailpit and disposable data. Real email delivery, MFA, independent backups, and tested restoration are required before serving real users.
