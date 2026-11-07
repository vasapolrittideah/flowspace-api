# Spec: Identity password login and sessions

Module ID: `identity-password-login-and-sessions`

Status: Draft

## Objective

Let a Flowspace account holder sign in with an email address and a password, use a separate access token and refresh token, and end 1 session or all sessions. Protected services must reject new requests from a revoked or expired session. The first users are API clients and the web frontend, and all data is disposable under [ADR-0022](../adr/0022-single-host-storage-holds-disposable-data.md).

This specification defines the shared session life cycle for password login and for the sessions that signup, account claim, provider login, and MFA login create. It also defines how Identity delivers the refresh token: in the JSON body for API clients, or in an HttpOnly cookie for browser clients. With the cookie, a page reload keeps the session, and no script can read the refresh token.

## Scope and ADRs

The scope covers password login, token issuance, refresh-token delivery in the body or in a cookie, refresh, logout of 1 session or all sessions, the public signing keys, the internal session check, and the browser request checks and Cross-Origin Resource Sharing (CORS) rules of Identity. These operations share 1 session record and its expiry and revocation rules.

This capability excludes these items:

- Password reset, provider login and link, email changes, session listing, and MFA. Their specifications use the session and delivery rules of this specification.
- The CORS rules of the other public services, which each service applies when it serves the web frontend.
- The remembered-device token of [ADR-0050](../adr/0050-remembered-devices-skip-the-mfa-login-step-for-30-days.md) in a browser.
- Workspace membership and role decisions.

These decisions apply:

- [ADR-0031](../adr/0031-flowspace-owns-authentication-and-revocable-sessions.md): Identity owns the token model, refresh rotation, reuse detection, and revocation.
- [ADR-0032](../adr/0032-identity-signup-recovers-with-login.md): password login recovers a lost signup or claim response.
- [ADR-0033](../adr/0033-identity-token-issuance-does-not-replay-responses.md): login and refresh reject `Idempotency-Key` and never replay a token pair.
- [ADR-0036](../adr/0036-authenticate-internal-session-checks-with-mutual-tls.md): `CheckSession` uses mutual TLS and a caller allowlist.
- [ADR-0052](../adr/0052-browser-sessions-keep-the-refresh-token-in-an-httponly-cookie.md): a browser client receives the refresh token in a `__Host-` HttpOnly cookie on the Identity hostname, and Identity checks the exact `Origin` of each request that uses the cookie. This is a scoped exception to the rule of ADR-0031 that clients send refresh tokens only to the refresh endpoint.

## Contract

Use package `flowspace.identity.v1` and service `IdentityService`.

The `use_refresh_token_cookie` field is a `bool` that selects cookie delivery of the refresh token. Its default, `false`, selects JSON delivery. Each RPC that issues a refresh token has this field: `CreateAccount`, `ClaimUnverifiedAccount`, `CreatePasswordSession`, `CreateProviderSession`, `CompleteMfaLogin`, and `RefreshSession`.

The refresh-token cookie is named `__Host-flowspace-refresh`. Its value is the opaque refresh token.

### Methods

| RPC | Public HTTP route | Request fields | Response fields |
| --- | --- | --- | --- |
| `CreatePasswordSession` | `POST /v1/password-sessions` | `email` (required), `password` (required), `use_refresh_token_cookie` | `subject`, `email_verified`, `access_token`, `refresh_token`, `access_token_expires_at`, `refresh_token_expires_at`, `session_expires_at` |
| `RefreshSession` | `POST /v1/session-refreshes` | `refresh_token`, `use_refresh_token_cookie` | `access_token`, `refresh_token`, `access_token_expires_at`, `refresh_token_expires_at`, `session_expires_at` |
| `LogoutCurrentSession` | `POST /v1/session-logouts` | None | `revoked` |
| `LogoutAllSessions` | `POST /v1/account-session-logouts` | None | `revoked` |
| `CheckSession` | None (internal) | `subject` (required), `session_id` (required) | `email_verified` |

### Method requirements

| RPC | Authentication | Retry |
| --- | --- | --- |
| `CreatePasswordSession` | None | Rejects `Idempotency-Key` under [ADR-0033](../adr/0033-identity-token-issuance-does-not-replay-responses.md) |
| `RefreshSession` | None | Rejects `Idempotency-Key` under [ADR-0033](../adr/0033-identity-token-issuance-does-not-replay-responses.md) |
| `LogoutCurrentSession` | Bearer access token | Safe to retry |
| `LogoutAllSessions` | Bearer access token | Safe to retry |
| `CheckSession` | Service mTLS under [ADR-0036](../adr/0036-authenticate-internal-session-checks-with-mutual-tls.md) | Safe to retry |

### HTTP-only endpoints

| Endpoint | Public HTTP route | Authentication | Request | Response |
| --- | --- | --- | --- | --- |
| Public signing keys | `GET /.well-known/jwks.json` on the internal HTTP port | None | None | The JSON Web Key Set (JWKS) with the public verification keys |
| CORS preflight | `OPTIONS` on each public Identity route | None | `Origin`, `Access-Control-Request-Method`, and `Access-Control-Request-Headers` headers | HTTP 204 with no body. The CORS headers only when the origin is allowed |

### Resource fields

Session tokens:

| Protobuf field | JSON field | Meaning |
| --- | --- | --- |
| `subject` | `subject` | The stable subject of the account, output only. Only password login returns it in this module |
| `email_verified` | `emailVerified` | The current verified-email state of the account, output only. Only password login returns it in this module |
| `access_token` | `accessToken` | The signed access token, output only |
| `refresh_token` | `refreshToken` | The opaque refresh token for JSON delivery, or an empty string for cookie delivery, output only |
| `access_token_expires_at` | `accessTokenExpiresAt` | The server time when the access token expires, output only |
| `refresh_token_expires_at` | `refreshTokenExpiresAt` | The server time when the refresh token expires, for JSON delivery and cookie delivery, output only |
| `session_expires_at` | `sessionExpiresAt` | The absolute expiry of the session, output only |

### Errors

| Condition | gRPC status | HTTP status |
| --- | --- | --- |
| Malformed request | `InvalidArgument` | 400 |
| Unknown email, retired account, or wrong password | `Unauthenticated` | 401 |
| Missing, unknown, expired, revoked, or rotated refresh token, in the body or in the cookie | `Unauthenticated` | 401 |
| Missing or invalid access token, or an inactive session | `Unauthenticated` | 401 |
| A request that uses the cookie fails the `Origin` or content type check | `PermissionDenied` | 403 |
| A limit is reached | `ResourceExhausted` | 429 |
| Identity, its database, or its limit store is unavailable | `Unavailable` | 503 |

### CORS headers

| Header | Value |
| --- | --- |
| `Access-Control-Allow-Origin` | The request `Origin`, only when it exactly matches an allowed origin |
| `Access-Control-Allow-Credentials` | `true`, only with `Access-Control-Allow-Origin` |
| `Access-Control-Allow-Methods` | `GET, POST, PATCH`, on a preflight response |
| `Access-Control-Allow-Headers` | `Authorization, Content-Type, Idempotency-Key, X-Request-ID`, on a preflight response |
| `Access-Control-Expose-Headers` | `X-Request-ID` |
| `Vary` | `Origin`, on each response of a public Identity route |

### Configuration

| Variable | Default | Meaning |
| --- | --- | --- |
| `ALLOWED_ORIGINS` | Unset | The exact frontend origins of the environment, separated by commas, such as `https://app.<domain>`. Each origin has a scheme, a host, and an optional port, with no path, no wildcard, and no `null`. Identity does not start when an entry is malformed. When it is unset, Identity sends no CORS headers and rejects each request that uses a cookie. |

## Behavior

### Password login

- Identity compares email addresses with the rules of the [Identity signup and email verification](identity-signup-and-email-verification.md) specification. It applies the same Unicode NFC normalization to the password before it checks the stored Argon2id hash.
- An existing account with the correct password receives 1 new device session and 1 token pair. The stable subject does not change. An unverified account can log in and call the Identity verification methods, but Workspace still denies it at its verified-email gate.
- An unknown account, a retired account, and a wrong password give the same public error. For an unknown account, Identity does the same password-hash work, so that the response time does not show whether the account exists.
- After it checks the request shape, Identity allows 60 password-login attempts for each trusted source address in a rolling hour, and 10 attempts for each normalized email in a rolling 15 minutes. Both limits use shared state and apply to active, retired, and unknown accounts. Only configured edge proxies can supply the source address.
- Each admitted attempt counts against both limits, including a successful login, and a success resets neither counter. When either limit is full, Identity returns `ResourceExhausted` before hashing and does not count the rejected request. Each counted attempt leaves its window on its own, so a denied request does not extend the wait. When the limit state is unavailable, Identity returns `Unavailable` and does not skip a limit.
- Identity limits concurrent password-hash work, so that a burst of allowed requests cannot use up its CPU or memory budget. It passes cancellation to the password hashing and the database work, and it reports success only after the required transaction commits.
- A lost login response can leave an extra session when the client logs in again. Identity stores only refresh-token hashes, so it cannot replay the first token pair.

### Token and session lifetime

- An access token expires 10 minutes after issue, or at the absolute expiry of the session if that comes first. The live session check can reject it earlier after logout, account retirement, or session expiry.
- Each session has an idle expiry 30 days after its creation or its last successful refresh. A successful refresh moves the idle expiry to 30 days after that refresh, but never past the absolute expiry.
- Each session has an absolute expiry 90 days after its creation. A refresh never moves it. At or after either expiry, Identity rejects refresh and session checks, even when an access token still passes local signature checks.
- Identity uses server time for each expiry decision. At the exact expiry time, the token or session is expired. The clock tolerance of the JWT check never extends the server-side session limits.
- The same lifetime rules apply to the sessions of signup, account claim, provider login, and MFA login.
- The access token is a signed JWT with a stable subject, a session ID, an issuer, an audience, an issue time, an expiry, and a token ID. Its type tells it apart from other tokens. Identity accepts only its configured signing algorithm and a public key bound to the configured issuer. It rejects a token with a missing or invalid claim, an unknown key ID, a wrong audience, a wrong type, or an expired lifetime.

### Refresh-token delivery

- JSON delivery puts the refresh token in the `refresh_token` field of the response and sets no cookie. API clients use it.
- Cookie delivery leaves the `refresh_token` field empty and sets the `__Host-flowspace-refresh` cookie in the same response. The `refresh_token_expires_at` field still shows the expiry when the response has it. Browser clients use it.
- The cookie has the `Secure`, `HttpOnly`, and `SameSite=Strict` attributes, the `Path=/` attribute, and no `Domain` attribute. Its `Max-Age` ends it at the expiry of the refresh token.
- A request with `use_refresh_token_cookie=true` selects cookie delivery for the token pair that it creates. The rule is the same for each RPC that the Contract section names. For `CreateAccount`, `ClaimUnverifiedAccount`, and `CompleteMfaLogin`, this rule replaces the rule of the [Identity signup and email verification](identity-signup-and-email-verification.md) and [Identity MFA login](identity-mfa-login.md) specifications that the response contains the refresh token.
- When a first step that selects cookie delivery returns an MFA challenge, the challenge stores that choice. A provider completion that reads the attempt cookie counts as a first step that selects cookie delivery. `CompleteMfaLogin` for that challenge then uses cookie delivery and the browser request checks, regardless of the value of `use_refresh_token_cookie`.
- When a `RefreshSession` request has an empty `refresh_token` field, Identity reads the refresh token from the cookie. Such a refresh always uses cookie delivery, regardless of the value of `use_refresh_token_cookie`. When the request has a nonempty `refresh_token` field, Identity ignores the cookie.
- A `RefreshSession` request with an empty `refresh_token` field and no cookie returns `Unauthenticated`.
- Identity reads the cookie only in `RefreshSession`. The browser sends the cookie with each Identity request that includes credentials, and the other routes ignore it.
- Each token-issuance response and each response that sets or clears the cookie has `Cache-Control: no-store`, so that no shared HTTP cache stores a token.

### Refresh and replay

- Identity creates each opaque refresh token from at least 256 random bits and stores only its hash. A successful refresh consumes the current token, records the hash of the new token, moves the idle expiry, and issues a new access token and refresh token for the same session, in 1 transaction.
- Identity keeps enough hashed token history to recognize a rotated token. A reuse of that token revokes its device session and the current refresh token of that session. An unknown token names no session to revoke.
- When 2 refreshes use 1 token at the same time, only 1 can succeed. The other sees a used token and revokes the session under ADR-0031. A client must send only 1 refresh at a time for a session.
- Tabs of 1 browser share 1 cookie. When 2 tabs refresh at the same time, Identity sees the second refresh as reuse and revokes the session. The web frontend must allow only 1 refresh at a time across tabs, such as with the Web Locks API.
- A client must not retry a refresh with the old token after a lost response. Once the first refresh commits, the retry counts as reuse and revokes the session. The same rule applies to a lost cookie response, because the browser keeps the old cookie. The client recovers through a new login. A response never replays a refresh token.
- A refresh creates no new session and never extends the 90-day absolute expiry. An expired or revoked session cannot refresh, even when the hash of the token matches a stored value.
- The rotation and reuse rules are the same for a refresh token in the body and in the cookie. A successful cookie refresh sets a new cookie with the new token.

### Logout and protected requests

- Before logout, Identity checks that the session of the caller is active. `LogoutCurrentSession` revokes the session of the access token. `LogoutAllSessions` revokes each session of the subject of the token, including the current one. Both commit the revocation and the refresh-token invalidation before they report success.
- Identity serializes an all-session logout with the session creation of the same subject. Sessions that commit before the logout are revoked. A login that commits after it can create a new active session.
- A session check that starts after the revocation commit rejects the session. A request admitted before the commit can finish.
- `LogoutCurrentSession`, `LogoutAllSessions`, and a `RefreshSession` request that fails with `Unauthenticated` always send an expired `__Host-flowspace-refresh` cookie. In a browser, the expired cookie removes the cookie. An API client ignores it.
- Each protected service first checks the access token locally, then calls `CheckSession` before it admits each protected request. It sends the subject and session ID of the token, and Identity compares both with its own session and account records.
- A successful `CheckSession` returns the current verified-email state. An unverified session is active but cannot use Workspace. An inactive session returns `Unauthenticated`. Identity never sends a workspace membership or role through this method.
- A protected service does not cache a successful session check. It denies the request when Identity returns an inactive result, times out, or cannot confirm the session, and it reports a temporary Identity failure as service unavailability.
- Logout needs a valid access token. When the token has expired, the client refreshes first or signs in again. When a logout response is lost, the client can retry while the token is valid. A later `Unauthenticated` response means that the token no longer proves an active session, but it does not prove which request revoked it.

### Browser request checks

- A request that sets `use_refresh_token_cookie=true` and a `RefreshSession` request with an empty `refresh_token` field must pass 2 checks before Identity does any other work. Identity returns `PermissionDenied` and changes no cookie when a check fails.
- The `Origin` check passes only when the `Origin` header is exactly equal to an origin in `ALLOWED_ORIGINS`, byte for byte. A missing `Origin` header and the value `null` fail the check.
- The content type check passes only when the media type is `application/json`, with or without parameters. This content type makes the browser send a CORS preflight request first, so a form on another site cannot send the request.
- Each public REST `POST` route of Identity returns `InvalidArgument` before its handler runs when the media type is not `application/json`. A form on another site then cannot reach logout or a failed refresh, so it cannot clear the cookie.
- Identity answers CORS requests only for the origins in `ALLOWED_ORIGINS`, with the headers of the CORS headers table. It answers a request from another origin without CORS headers, so the browser blocks the response.
- Identity is the only public service that allows credentials, because only Identity reads or sets cookies. The frontend includes credentials only in the requests that read or set a cookie: the token-issuance RPCs with cookie delivery, `RefreshSession`, both logout RPCs, and the provider start and completion requests that use an attempt cookie.
- In staging and production, Identity has its own hostname, such as `auth.<domain>`, under the same registrable domain as the frontend, such as `app.<domain>`. The browser then treats the cookie as a first-party cookie and sends it to no other service. The Identity hostname serves only Identity routes and the callback page, with no user content and no third-party scripts.
- Locally, `localhost` on different ports is the same site. Local development supports only browsers that treat `http://localhost` as a secure origin for `Secure` cookies, such as Chrome and Firefox.
- A frontend on another site, such as a Vercel preview domain, gets no cookie. It can still sign in with cookie delivery, but a reload needs a new login.
- The public route passes the `Cookie` request header to Identity and returns the `Set-Cookie` response header that Identity sets.

### Signing keys

- Identity signs access tokens with a private key that protected services cannot read. Verifiers select public keys from the configured Identity JWKS, reject unknown key IDs, and never fetch a key from a URL in a token.
- A routine rotation publishes a new public key before Identity signs with it. Identity keeps an old public key until each access token signed with it has expired, including the cache and clock margin. An emergency rotation needs a separate incident procedure.
- The JWKS never contains a private key or refresh-token material.

### Data and compatibility

- The change adds no column to the session tables. Cookie delivery stores the same refresh-token hash as JSON delivery. The MFA challenge stores the delivery choice of its first step, and the migration of the `identity-mfa-login` module adds that column, because MFA login has no table yet.
- `use_refresh_token_cookie` is a new optional field, and the `refresh_token` field of `RefreshSessionRequest` stops being required. An API client that does not send the new field keeps JSON delivery, so its behavior does not change.
- Older code ignores `use_refresh_token_cookie` and returns the refresh token in the body. Before a rollback to older code, the web frontend stops sending the field. A browser that keeps a cookie after the rollback gets `InvalidArgument` for a refresh with an empty `refresh_token` field, and the user signs in again.

### Diagnostics

Identity records the outcome of login, refresh, reuse, and logout, the failed-login and refresh-reuse counts, session-check failures, and Identity unavailability. Metrics use no high-cardinality identifiers. Logs, traces, metrics, and errors must never contain these values:

- Passwords, access tokens, refresh tokens, and private keys.
- The `Cookie` and `Set-Cookie` headers and the `Location` header of a provider callback redirect, in Identity, the public route, the edge, and the tunnel, and in local logs.
- Full email addresses.

## Testing strategy

| Risk | Test level |
| --- | --- |
| A login, signup, or recovery response shows whether an account exists, by its content, status, or timing (`ID-T01`) | Unit, Integration |
| A password-login limit misses its 61st source attempt or 11th email attempt, or a denied request extends a window (`ID-T02`, `ID-T16`) | Integration |
| A token with a wrong algorithm, issuer, audience, type, key ID, signature, subject, session ID, or expiry passes (`ID-T09`, `ID-T10`) | Unit |
| A stolen or rotated refresh token, in the body or in the cookie, creates new tokens (`ID-T11`) | Integration |
| A cookie refresh rotates differently from a body refresh, or 2 concurrent refreshes of 1 token both succeed (`ID-T11`, `ID-T12`) | Integration |
| Logout reports success before the revocation commits, or logout or a failed refresh leaves the cookie in the browser (`ID-T13`) | Integration |
| A protected service admits a request without a live session check, or after Identity fails (`ID-T14`) | Integration, Cluster |
| A malformed request, a duplicate security header, or a rejected `Idempotency-Key` header reaches the token logic (`ID-T15`) | Unit |
| Simultaneous password hashes go over the process budget (`ID-T16`) | Integration |
| A token, a password, or a `Cookie` or `Set-Cookie` header appears in telemetry or errors (`ID-T17`) | Integration, Cluster |
| A session or token outlives its 10-minute, 30-day, or 90-day boundary, or the cookie outlives the refresh token (`ID-T19`) | Unit |
| A cookie request from another origin, with no `Origin`, with `Origin: null`, or with a form content type uses the cookie (`ID-T23`) | Unit, Integration |
| A response sets the cookie without `__Host-`, `Secure`, `HttpOnly`, `SameSite=Strict`, `Path=/`, or with a `Domain` attribute (`ID-T23`, `ID-T24`) | Unit |
| A CORS response allows a wrong origin, allows credentials on another service, or has no `Vary: Origin` header (`ID-T23`, `ID-T25`) | Unit, Cluster |
| A `text/plain` or form-encoded `POST` from another site reaches refresh or logout and clears the cookie (`ID-T23`) | Unit |
| A cookie-delivery first step completes MFA with the refresh token in the body (`ID-T26`) | Integration |
| A cookie delivery response puts the refresh token in the body (`ID-T26`) | Unit |
| A browser loses the session after a reload, or the public route drops the `Cookie` or `Set-Cookie` header | Cluster |

Use a controlled clock instead of sleeps for expiry tests.

## Implementation boundaries

Always do these actions:

- Keep separate access and refresh tokens, single-use rotation, and a live session check for each protected request.

Ask the maintainer before these actions:

- Change the 10-minute access lifetime, the 30-day idle lifetime, the 90-day absolute lifetime, or the rule that a reuse revokes the session.
- Change the signing algorithm, the issuer or audience values, the JWT clock tolerance, the JWKS route, or the key rotation procedures.
- Change the 60-attempt source limit, the 10-attempt email limit, or either rolling window.
- Change a cookie attribute or name, or let another route read the refresh-token cookie.
- Allow a wildcard origin, or allow credentials in a public service other than Identity.

Never do these actions:

- Do not trust a subject, session ID, verified-email state, or workspace role from the client.
- Do not accept a refresh token as a bearer access token, or put either token in a URL, a log, a trace, or a shared cache.
- Do not admit a protected request from a locally valid JWT without a successful live session check.

## Success criteria

1. Given an existing account with a password, When the user sends the correct email and password, Then Identity creates 1 session and returns the stable subject, the current email state, 2 different tokens, and their expiry times.
2. Given a password login commits but its response is lost, When the client logs in again, Then Identity creates a new session without a replay of the first token pair, and the first session can stay active.
3. Given a signup or claim response is lost but the user knows the current password, When the user logs in, Then Identity issues a new session for the same active subject without a replay of an earlier refresh token.
4. Given an unknown or retired account or a wrong password, When a client logs in, Then the same safe `Unauthenticated` response comes back without a session or a token.
5. Given a source sends its 61st login attempt in an hour or an email receives its 11th attempt in 15 minutes, When Identity handles the attempt, Then Identity returns `ResourceExhausted` before hashing, the denied request extends neither window, and login becomes available as counted attempts leave their windows.
6. Given a correct password of an unverified account, When the user logs in and calls Workspace, Then login succeeds, the Identity verification methods stay available, and Workspace denies access until the email is verified.
7. Given a valid refresh token before both session limits, When a client uses it, Then Identity consumes it once and returns a new token pair for the same session with a later idle expiry and the same absolute expiry.
8. Given the 30-day idle limit or the 90-day absolute limit is reached, When a client uses the refresh token, Then the refresh fails without tokens and does not extend the session.
9. Given a refresh token has rotated, including after a concurrent refresh, When a client uses the old token again, Then Identity revokes that session, the current refresh token and later protected requests fail, and the other device sessions stay active.
10. Given a refresh commits but its response is lost, When the client sends the old token again, Then Identity treats it as reuse, and the client must log in to get a new session.
11. Given an active device session, When the user logs out that device, Then the revocation commits before success, a later request of that session fails even with a locally valid access token, and the other device sessions stay active.
12. Given active device sessions, When the user logs out all devices, Then each existing session fails live checks after the commit, and a later login can create a new session.
13. Given a protected service cannot confirm the session state with Identity, When it handles a new request, Then it denies the request and returns a temporary service failure without a cached active result.
14. Given a token issued before the email became verified, When the next live session check runs, Then it returns the verified state without new tokens.
15. Given old access tokens are still valid, When Identity rotates its signing key, Then verifiers accept both valid key IDs until the old tokens expire, and then reject the retired key.
16. Given a browser on an allowed origin, When it logs in with `use_refresh_token_cookie=true`, Then the response has an empty `refresh_token` field and sets the `__Host-flowspace-refresh` cookie with the `Secure`, `HttpOnly`, `SameSite=Strict`, and `Path=/` attributes and no `Domain` attribute.
17. Given a browser with a valid refresh-token cookie reloads the page, When it calls `RefreshSession` with an empty `refresh_token` field, Then Identity rotates the token, sets a new cookie, and returns a new access token with an empty `refresh_token` field.
18. Given a request that uses the cookie has an `Origin` that is not in `ALLOWED_ORIGINS`, no `Origin`, `Origin: null`, or a content type other than `application/json`, When Identity handles it, Then Identity returns `PermissionDenied`, reads no cookie, and issues no token.
19. Given a browser with a refresh-token cookie, When it logs out, logs out all sessions, or gets `Unauthenticated` from `RefreshSession`, Then the response sends an expired cookie, and the browser has no refresh-token cookie after it.
20. Given an allowed origin sends a CORS preflight request to Identity, When Identity answers, Then the response allows only that origin, allows credentials, and has the `Vary: Origin` header, and a preflight from another origin gets no CORS headers.
