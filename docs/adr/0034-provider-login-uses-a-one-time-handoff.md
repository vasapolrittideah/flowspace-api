# ADR-0034: Provider login uses a one-time handoff

## Status

Accepted

## Date

2026-09-24

## Context

The first FlowSpace clients are API clients. Google and GitHub return authorization results to a browser callback, while the API client needs a FlowSpace access token and refresh token. Browser token storage belongs to a later web design. [ADR-0005](0005-one-protobuf-contract-generates-rest.md) makes Protobuf the source of truth for public synchronous APIs.

[ADR-0011](0011-idempotency-keys-protect-non-idempotent-creates.md) requires response replay for non-idempotent creates. [ADR-0031](0031-flowspace-owns-authentication-and-revocable-sessions.md) requires Identity to store only refresh-token hashes. Identity cannot replay a lost token response without retaining a reusable refresh token.

## Decision

`StartProviderLogin` returns a provider authorization URL and a secret attempt token to the API client. Identity validates the provider callback result and displays a separate one-time handoff code in a browser page. The client copies that code and submits it with the attempt token to `CreateProviderSession`. The callback page and redirects contain no FlowSpace access or refresh token.

The provider callback is a small HTTP adapter. It accepts the provider response, delegates validation and state changes to Identity, and renders the handoff page. It does not define another client-facing account or session API. `StartProviderLogin` and `CreateProviderSession` remain Protobuf RPCs with generated REST/JSON routes. This callback adapter is a scoped exception to ADR-0005.

`StartProviderLogin` and `CreateProviderSession` reject `Idempotency-Key` and do not replay successful responses. This is a scoped exception to ADR-0011. If the start response is lost, the client starts a new attempt and the old one expires. If the session response is lost, the client completes a new provider login. The first session can remain active until expiry or logout. Identity never stores a refresh token in a form that it can replay.

## Alternatives Considered

### Return FlowSpace tokens through the callback

- Pros: The user does not copy a code between the browser and API client.
- Cons: A browser page or redirect would carry a reusable access or refresh token.
- Rejected: The API client receives tokens only after it presents both handoff proofs to Identity.

### Redirect to a client-owned loopback receiver

- Pros: A native client can receive the authorization result without manual copying.
- Cons: The initial API-client workflow would need a client-specific local receiver and redirect registration.
- Rejected: A manual handoff meets the first workflow without selecting a browser or native-client design for later releases.

### Store token responses for exact replay

- Pros: A client can recover a lost `CreateProviderSession` response without another provider login.
- Cons: Identity would retain a reusable refresh token and break the hash-only storage rule in ADR-0031.
- Rejected: A lost response requires a new provider login.

## Consequences

- API-client users copy a short-lived code from the callback page. A later web client needs its own reviewed token-handling design.
- A lost response can leave an attempt or session that the client did not observe. Attempts expire, while sessions follow the normal expiry and logout rules.
- The [provider-login spec](../specs/identity-provider-login.md) defines expiry, proof checks, email handling, limits, and tests for this handoff.
