# Spec: Identity password login and sessions

Module id: `identity-password-login-and-sessions`

Status: Approved.

## Objective

Allow a FlowSpace account holder to sign in with an email address and password, use a separate access and refresh token, and end one or all sessions. Protected services must reject new requests from a revoked or expired session. The first clients are API clients using disposable data.

This spec defines the shared session lifecycle for password login and for sessions created by signup or account claim. It does not report implementation progress or readiness for real users.

## Scope and decision sources

The scope covers password login, token issuance, refresh, one-session logout, all-session logout, public signing keys, and an internal session check. These operations share one session record and its expiry and revocation rules.

[ADR-0031](../adr/0031-flowspace-owns-authentication-and-revocable-sessions.md) owns the token and revocation model. [ADR-0032](../adr/0032-identity-signup-recovers-with-login.md) requires password login to recover a lost signup or claim response. [ADR-0033](../adr/0033-identity-token-issuance-does-not-replay-responses.md) defines login and refresh retry behavior. The [signup and verification spec](identity-signup-and-email-verification.md) owns password hashing and email comparison. The [Identity threat model](../security/identity-threat-model.md) defines the abuse cases referenced below. Protobuf and generated REST follow [ADR-0005](../adr/0005-one-protobuf-contract-generates-rest.md), [ADR-0007](../adr/0007-version-apis-by-compatibility-boundary.md), and [ADR-0009](../adr/0009-canonical-grpc-errors-map-to-http.md).

Password reset, provider login and linking, email changes, session listing, and MFA are outside this capability. Browser token storage and cross-site request protections belong to the later web specification. Later login methods must use the session lifetime and revocation rules in this spec. Workspace still owns membership and role decisions.

## Contract

Use package `flowspace.identity.v1` and the `IdentityService` contract. Public RPCs expose generated REST/JSON under `/v1`; `CheckSession` is an internal RPC without a public HTTP route. Source Protobuf definitions will fix the final field and method names.

| RPC | Public HTTP route | Request | Successful response |
| --- | --- | --- | --- |
| `CreatePasswordSession` | `POST /v1/password-sessions` | `email`, `password` | Stable subject, current `email_verified`, access token, refresh token, and expiry times |
| `RefreshSession` | `POST /v1/session-refreshes` | `refresh_token` | New access token, new refresh token, and expiry times |
| `LogoutCurrentSession` | `POST /v1/session-logouts` | Bearer access token, no body fields | Confirmation that the current session was revoked |
| `LogoutAllSessions` | `POST /v1/account-session-logouts` | Bearer access token, no body fields | Confirmation that every session for the subject was revoked |
| `CheckSession` | Internal RPC only | Validated `subject` and `session_id` from a protected service | Active session and current `email_verified` state |

Password login and refresh do not require an access token and reject the `Idempotency-Key` header under ADR-0033. Refresh tokens appear only in the `RefreshSession` request body and token-issuance responses. Logout methods require exactly one `Authorization: Bearer <access_token>` header. The authenticated token supplies the subject and current session ID; clients cannot choose a target subject or session. `CheckSession` requires an authenticated service caller. [ADR-0036](../adr/0036-authenticate-internal-session-checks-with-mutual-tls.md) defines mutual TLS for this call.

Each successful token-issuance response contains `access_token`, `refresh_token`, `access_token_expires_at`, `refresh_token_expires_at`, and `session_expires_at`. Password login also returns `subject` and `email_verified`. Refresh does not return an email address or workspace role. Tokens are secrets and responses must prevent storage by shared HTTP caches.

The access token is a signed JWT. It carries a stable subject, session ID, issuer, audience, issue time, expiry, and token ID. Its type distinguishes it from other tokens. The service accepts only its configured signing algorithm and a public key bound to the configured issuer. It rejects a token with a missing or invalid claim, unknown key ID, wrong audience, wrong type, or expired lifetime. The exact algorithm, issuer and audience values, clock tolerance, and JWKS transport details remain open before implementation.

Identity publishes the public verification keys as a JSON Web Key Set (JWKS). Only Identity uses the private key. A verifier can cache public keys under the later key-rotation policy, but it must check live session state on every protected request. JWKS never contains a private key or refresh-token material.

Use canonical gRPC errors and the gateway's default HTTP mapping. Malformed public input uses `InvalidArgument` (HTTP 400). An unknown email and a wrong password produce the same `Unauthenticated` (HTTP 401) response. An unknown, expired, or revoked refresh token also produces `Unauthenticated` without revealing session state. A known rotated refresh token produces the same error and revokes its session. An inactive `CheckSession` result uses `Unauthenticated`; an active result returns the current `email_verified` value. A request limit uses `ResourceExhausted` (HTTP 429). An unavailable session store or Identity service uses `Unavailable` (HTTP 503), without admitting a protected request. Safe errors never contain tokens, credentials, account state, or internal diagnostics.

## Required behavior

### Password login

- Identity compares email addresses using the rules in the signup spec. It applies the same Unicode NFC normalization to the supplied password before checking its stored Argon2id hash.
- An existing account with the correct password receives one new device session and one token pair. The stable subject does not change. An unverified account can log in and call Identity verification methods, but protected Workspace access still fails its verified-email gate.
- An unknown account, a retired account, and a wrong password produce the same public error. Identity performs a password-hash workload for an unknown account so the response does not reveal account existence through a practical timing difference.
- After checking request shape, Identity allows 60 password-login attempts per trusted source address in a rolling hour. It also allows 10 attempts per normalized email identifier in a rolling 15 minutes. Both limits use shared state and apply to active, retired, and unknown accounts. Only configured edge proxies can supply the source address.
- Every admitted attempt counts against both limits, including a successful login. Success does not reset either counter. If either limit is full, Identity returns `ResourceExhausted` before hashing and does not count the rejected request. Each counted attempt leaves its window individually, so a denied request cannot extend the wait. If limit state is unavailable, Identity returns `Unavailable` and does not bypass either limit.
- Identity bounds concurrent password-hash work so a burst of allowed requests cannot exhaust its CPU or memory budget.
- A lost successful login response can create an extra session if the client logs in again. Identity stores only refresh-token hashes, so it cannot replay the first token pair under ADR-0033.

### Token and session lifetime

- An access token expires ten minutes after issue, or when the session reaches its absolute expiry, whichever comes first. The live session check can reject it earlier after logout, account retirement, or session expiry.
- Each session has an idle expiry 30 days after its creation or last successful refresh. A successful refresh moves idle expiry to 30 days after that refresh, capped by the absolute expiry.
- Each session has an absolute expiry 90 days after its creation. Refresh never moves that absolute expiry. At or after either expiry, Identity rejects refresh and session checks even if an access token still passes local signature validation.
- Identity uses server time for every expiry decision. At the exact expiry instant, the token or session is expired. The clock tolerance for JWT validation remains open before implementation and must not extend the server-side session limits.
- The same lifetime rules apply to sessions created by `CreateAccount` and `ClaimUnverifiedAccount`. A later provider-login spec must apply them to provider sessions as well.

### Refresh and replay

- Identity generates an opaque refresh token from at least 256 random bits and stores only its hash. A successful refresh atomically consumes the current token, records the new token hash, advances idle expiry, and issues a new access token and refresh token for the same session.
- Identity keeps enough hashed token history to recognize a rotated token. Reuse of that token revokes the affected device session and its current refresh token. An unknown token does not identify a session to revoke.
- Two concurrent refreshes using one token cannot both succeed. One transition commits; the other observes a used token and revokes the session under ADR-0031. A client must serialize refresh requests for one session.
- A client must not automatically retry a refresh with the old token after a lost response. Once the first refresh commits, that retry counts as reuse and revokes the session. The client can recover through password login. The response never replays a refresh token.
- Refresh does not create a new session or extend its 90-day absolute expiry. Expired or revoked sessions cannot refresh, even when the presented token hash matches a stored value.

### Logout and protected requests

- Identity checks that the caller's session is active before logout. `LogoutCurrentSession` revokes the session named by the caller's validated access token. `LogoutAllSessions` revokes every existing session for that token's subject, including the current one. Both commit revocation and refresh-token invalidation before reporting success.
- Identity serializes all-session logout with session creation for the same subject. Sessions committed before logout are revoked. A password login committed afterward can create a new active session.
- A session check that starts after the revocation commit rejects the session. A request admitted before the commit can finish. A new password login after all-session logout creates a new session normally.
- Each protected service first verifies the access token locally, then calls `CheckSession` before admitting every protected request. It supplies the token subject and session ID, and Identity compares both with its own session and account records.
- A successful `CheckSession` returns the current verified-email state. An unverified session is active but cannot use Workspace. Identity never supplies a workspace membership or role through this method.
- A protected service does not cache a successful session check. It denies admission when Identity returns an inactive result, times out, or cannot confirm the session. A temporary Identity failure is reported as service unavailability instead of granting access.
- Logout uses a currently valid access token. If the token expired, the client refreshes first or signs in again. If a logout response is lost, the client can retry while the token remains valid; a later `Unauthenticated` response means the token no longer proves an active session, but does not prove which earlier request caused revocation.

### Signing keys and diagnostics

- Identity signs access tokens with a private key that protected services cannot read. Verifiers select public keys from a configured Identity JWKS, reject unknown key IDs, and never fetch a key from a URL supplied by a token.
- Routine rotation publishes a replacement public key before Identity signs with it. Identity retains an old public key until every access token signed with it has expired, including the approved cache and clock margin. Emergency rotation needs a separate incident procedure.
- Identity records login, refresh, replay, and logout outcomes without passwords, tokens, full email addresses, or private keys. Metrics include failed-login and refresh-replay counts, session-check failures, and Identity unavailability without high-cardinality identifiers.
- The ordinary five-second request cap and shorter caller deadlines apply to public and internal RPCs under [ADR-0010](../adr/0010-cap-ordinary-unary-requests-at-five-seconds.md). Identity propagates cancellation to password hashing and database work where possible and does not report success before a required transaction commits.

## Commands

Run commands from the repository root during implementation. Approval does not mean that Identity code or contracts exist.

| Purpose | Command |
| --- | --- |
| Compile Identity binaries | `go build ./services/identity/cmd/...` |
| Run Identity tests | `go test ./services/identity/...` |
| Run Identity integration tests | `go test -tags=integration ./services/identity/...` |
| Lint source contracts | `task buf -- lint` |
| Generate API code | `task buf -- generate` |
| Check API compatibility against main | `task buf -- breaking --against '.git#branch=main'` |
| Generate query code | `task sqlc -- generate` |

## Testing strategy

Unit tests cover password normalization, public error classification, token claims, and the exact ten-minute, 30-day, and 90-day boundaries. PostgreSQL integration tests cover atomic login, token rotation, concurrent refresh, replay revocation, both logout scopes, account retirement, and session expiry. Use a controlled clock rather than sleeps for expiry tests.

Transport tests cover malformed input, rejected `Idempotency-Key` headers, duplicate security headers, generic login errors, absent or invalid access and refresh tokens, both rolling login limits, deadlines, cancellation, and safe error bodies. Limit tests cover the 60th and 61st source attempts, the 10th and 11th email attempts, successful and unknown-account attempts, and rejected requests that do not extend either window. Load tests prove that simultaneous password hashes stay within the process budget. Capture logs and traces to prove that no reusable secret appears. Token tests cover the wrong algorithm, issuer, audience, type, key ID, signature, subject, session ID, and expiry.

Cross-service tests prove that Workspace rejects an unverified account, a revoked device, an all-session logout, and an expired session. They also prove that Identity unavailability denies new protected requests. Test signing-key overlap and retirement after the final accepted token expires.

Enumeration and password-guessing tests cover ID-T01 and ID-T02. Token and signing-key tests cover ID-T09, ID-T10, and ID-T19. Refresh and concurrency tests cover ID-T11 and ID-T12. Logout and session-check tests cover ID-T13 and ID-T14. Input, load, and telemetry tests cover ID-T15, ID-T16, and ID-T17 in the [threat model](../security/identity-threat-model.md). The host-compromise exercise for ID-T22 remains a separate real-user gate.

## Boundaries

### Always

- Preserve separate access and refresh tokens, single-use rotation, and live session checks for protected requests.
- Enforce both session expiry limits on the server and keep refresh history sufficient to detect replay.
- Make revocation durable before logout success and use shared state for login limits when replicas scale.
- Use mutual TLS and the caller allowlist from ADR-0036 for `CheckSession`.

### Ask first

- Obtain approval before changing the 10-minute access lifetime, 30-day idle lifetime, 90-day absolute lifetime, or replay-revokes-session rule.
- Before implementing token issuance or verification, approve the signing algorithm, issuer and audience values, and JWT clock tolerance.
- Before implementing key publication or rotation, approve the JWKS route, cache bounds, and routine and emergency key procedures.
- Obtain approval before changing the 60-attempt source limit, the 10-attempt email limit, or either rolling window.

### Never

- Do not trust a client-supplied subject, session ID, email verification state, or workspace role.
- Do not accept a refresh token as a bearer access token or place either token in a URL, log, trace, or shared cache.
- Do not admit a protected request from a locally valid JWT without a successful live session check.

## Success criteria

Each row describes an observable result required before implementation can claim completion. The last column links the result to the applicable threat IDs.

| Given | Then | Threat |
| --- | --- | --- |
| An existing account sends its correct email and password. | Identity creates one session and returns the stable subject, current email state, two distinct tokens, and their expiry times. | ID-T02, ID-T09, ID-T11 |
| A client loses a committed password-login response and logs in again. | Identity creates a new session without replaying the first token pair; the first session can remain active. | ID-T11 |
| A signup or claim response is lost, but the user knows the current password. | Password login issues a new session for the same active subject without replaying a prior refresh token. | ID-T11 |
| An unknown or retired account, or a wrong password, is used for login. | The same safe `Unauthenticated` response appears without a session or token. | ID-T01, ID-T02 |
| A source sends a 61st login attempt within an hour, or an email identifier receives an 11th attempt within 15 minutes. | Identity returns `ResourceExhausted` before hashing. The denied request extends neither window, and login becomes available as counted attempts age out. | ID-T01, ID-T02, ID-T16 |
| A correct password belongs to an unverified account. | Login succeeds, Identity verification remains available, and Workspace denies access until the email is verified. | ID-T14 |
| A valid current refresh token is used before either session limit. | Identity consumes it once and returns a new token pair for the same session with a later idle expiry and unchanged absolute expiry. | ID-T11, ID-T12 |
| A refresh token is used at the 30-day idle limit or the 90-day absolute limit. | Refresh fails without issuing tokens or extending the session. | ID-T11, ID-T19 |
| A known rotated refresh token is used again, including after a concurrent refresh. | Identity revokes that session; the current refresh token and later protected requests fail. Other device sessions remain active. | ID-T11, ID-T12 |
| A client loses a committed refresh response and sends the old token again. | Identity treats it as reuse; the client must log in to establish a new session. | ID-T11, ID-T12 |
| A user logs out the current device. | Revocation commits before success, and a later request from that session fails even with a locally valid access token. Other device sessions remain active. | ID-T09, ID-T13 |
| A user logs out every device. | All existing sessions stop passing live checks after the commit; a later password login can create a new session. | ID-T09, ID-T13 |
| A protected service cannot confirm session state with Identity. | It denies the new request and returns a temporary service failure without using a cached active result. | ID-T14, ID-T16 |
| The email becomes verified after a token is issued. | The next live session check reports the verified state without requiring new tokens. | ID-T14 |
| Identity rotates its signing key while old access tokens remain valid. | Verifiers accept both valid key IDs until old tokens expire, then stop accepting the retired key. | ID-T10 |
| The capability is submitted for implementation review. | Contract, database, abuse, concurrency, key-rotation, and cross-service tests pass under repository quality checks. | ID-T15, ID-T17, ID-T20 |
