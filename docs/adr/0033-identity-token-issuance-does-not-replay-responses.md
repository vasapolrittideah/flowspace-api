# ADR-0033: Identity token issuance does not replay responses

## Status

Accepted

## Date

2026-09-23

## Context

[ADR-0011](0011-idempotency-keys-protect-non-idempotent-creates.md) requires an `Idempotency-Key` and exact response replay for non-idempotent creates. Password login creates a session, and refresh replaces a token pair. [ADR-0031](0031-flowspace-owns-authentication-and-revocable-sessions.md) stores only refresh-token hashes and revokes a session when a rotated token is reused. Exact replay would require Identity to retain a reusable refresh token.

An API client can lose either response after Identity commits. Repeating password login can create another session. Repeating refresh with the old token can trigger reuse detection and require a new login.

## Decision

`CreatePasswordSession` and `RefreshSession` reject the `Idempotency-Key` header. These methods never promise an exact response replay. This is a scoped exception to ADR-0011. Other protected creates still follow that record unless another accepted exception applies.

If a password-login response is lost, the client can submit the credentials again. A successful second request creates another session. The first session can remain active until it expires or is revoked.

Refresh consumes its current token atomically. If the response is lost, the client must not retry with the old token. A known rotated token counts as reuse and revokes the affected session, including when the reuse follows a lost response or a concurrent refresh. The client recovers through password login.

## Alternatives Considered

### Store token pairs for exact replay

- Pros: A retry with the same key can return the first response.
- Cons: Identity retains a reusable refresh token that a storage breach can expose.
- Rejected: Refresh-token storage remains hash-only under ADR-0031.

### Return a new token pair for a repeated refresh

- Pros: A client can recover without signing in again.
- Cons: The new response differs from the first response and treats reuse of a rotated token as normal.
- Rejected: A lost refresh response requires a new login.

## Consequences

- A repeated password login can leave an additional active session that the client did not observe.
- A lost refresh response ends that device session if the client retries the old token. Clients must serialize refresh requests for each session.
- Identity stores no refresh token in a form that it can replay. This exception does not change ADR-0011 for other protected creates.
