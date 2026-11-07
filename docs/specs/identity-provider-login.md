# Spec: Identity provider login

Module ID: `identity-provider-login`

Status: Draft

## Objective

Let a Flowspace user sign in with Google or GitHub. Identity creates a Flowspace session only after it checks the provider response. A first-time user can get a new provider-only account when no Flowspace account owns the verified email address. The first users are API clients and the web frontend, and all data is disposable under [ADR-0022](../adr/0022-single-host-storage-holds-disposable-data.md).

An API client opens the provider page in a browser, copies a one-time handoff code from the Identity callback page, and sends it with its secret attempt token. A browser client does not copy a code: Identity keeps the attempt token in an HttpOnly cookie and redirects the callback to the frontend with the handoff code in the URL fragment.

## Scope and ADRs

The scope covers provider authorization, callback checks, the handoff to an API client or a browser client, account lookup or creation, and session issuance. Google and GitHub are the only providers.

This capability excludes these items:

- Provider link and unlink for a signed-in user. Their own specifications define them.
- Email changes, MFA, and other providers.
- A recovery path for a provider-only user who loses the provider account.

These decisions apply:

- [ADR-0031](../adr/0031-flowspace-owns-authentication-and-revocable-sessions.md): Identity owns provider identities, email verification, and Flowspace sessions.
- [ADR-0034](../adr/0034-provider-login-uses-a-one-time-handoff.md): the callback adapter shows a one-time handoff code to an API client, and `StartProviderLogin` and `CreateProviderSession` reject `Idempotency-Key`.
- [ADR-0052](../adr/0052-browser-sessions-keep-the-refresh-token-in-an-httponly-cookie.md): a browser client receives the refresh token in a cookie, and Identity checks the exact `Origin` of each request that uses a cookie.
- [ADR-0053](../adr/0053-browser-provider-flows-redirect-the-handoff-code-to-the-frontend.md): for an attempt with a return URL, Identity keeps the attempt token in a `__Host-` cookie and redirects the callback to the frontend with the handoff code in the fragment. This is a scoped exception to ADR-0034.

## Contract

Use package `flowspace.identity.v1` and service `IdentityService`.

The `provider` field uses the `Provider` enum and accepts only `PROVIDER_GOOGLE` and `PROVIDER_GITHUB`. `PROVIDER_UNSPECIFIED` and unknown values are malformed. The `use_refresh_token_cookie` field follows the [Identity password login and sessions](identity-password-login-and-sessions.md) specification.

The login attempt cookie is named `__Host-flowspace-login-attempt`. Its value is the attempt token.

### Methods

| RPC | Public HTTP route | Request fields | Response fields |
| --- | --- | --- | --- |
| `StartProviderLogin` | `POST /v1/provider-login-attempts` | `provider` (required), `return_url` | `authorization_url`, `attempt_token`, `attempt_expires_at` |
| `CreateProviderSession` | `POST /v1/provider-sessions` | `attempt_token`, `handoff_code` (required), `use_refresh_token_cookie` | `subject`, `email_verified`, `access_token`, `refresh_token`, `access_token_expires_at`, `refresh_token_expires_at`, `session_expires_at` |

### Method requirements

| RPC | Authentication | Retry |
| --- | --- | --- |
| `StartProviderLogin` | None | Rejects `Idempotency-Key` under [ADR-0034](../adr/0034-provider-login-uses-a-one-time-handoff.md) |
| `CreateProviderSession` | None | Rejects `Idempotency-Key` under [ADR-0034](../adr/0034-provider-login-uses-a-one-time-handoff.md) |

### HTTP-only endpoints

| Endpoint | Public HTTP route | Authentication | Request | Response |
| --- | --- | --- | --- | --- |
| Provider callback | `GET /v1/provider-login-callbacks/{provider}` | None | The `code` and `state` query parameters, or the `error` query parameter of the provider | For an attempt without a return URL, an HTML page with the handoff code or a safe error. For an attempt with a return URL, an HTTP 303 redirect to the return URL with `#handoff_code=<code>` or `#error=<callback error code>`. No Flowspace token |

### Resource fields

Login attempt:

| Protobuf field | JSON field | Meaning |
| --- | --- | --- |
| `authorization_url` | `authorizationUrl` | The provider authorization URL that the user opens in a browser, output only |
| `attempt_token` | `attemptToken` | The secret token of the attempt, at least 128 random bits, or an empty string when the request has a return URL, output only |
| `attempt_expires_at` | `attemptExpiresAt` | The server time when the attempt expires, 10 minutes after its start, output only |

`CreateProviderSession` returns the session token fields of the Identity password login and sessions specification. It returns no provider token.

### Errors

| Condition | gRPC status | HTTP status |
| --- | --- | --- |
| Malformed request, or a `return_url` that is not on the configured list | `InvalidArgument` | 400 |
| The attempt is unknown, pending, expired, used, or failed, or the handoff code is wrong | `Unauthenticated` | 401 |
| A new provider identity has no usable verified email, or its email belongs to another Flowspace account | `FailedPrecondition` | 400 |
| A request with a return URL or a completion that reads the attempt cookie fails the `Origin` or content type check | `PermissionDenied` | 403 |
| A limit is reached | `ResourceExhausted` | 429 |
| Identity, its database, its limit store, or the provider is unavailable | `Unavailable` | 503 |

The `FailedPrecondition` error has 1 message for both causes, with the same response shape and practical timing: "Unable to complete provider login. Check the provider email or sign in through another method to link the provider."

### Callback error codes

| Code | Meaning |
| --- | --- |
| `failed` | The user denied consent, or the provider proof failed its checks. The attempt has ended. |
| `unavailable` | A provider call or an Identity dependency failed. The attempt has ended. |

### Configuration

| Variable | Default | Meaning |
| --- | --- | --- |
| `PROVIDER_RETURN_URLS` | Unset | The exact frontend callback URLs of the environment, separated by commas, such as `https://app.<domain>/auth/callback`. Each URL has no fragment, and its origin is in `ALLOWED_ORIGINS`. Identity does not start when an entry breaks these rules. Identity compares a `return_url` with each entry byte for byte. When it is unset, Identity rejects each request that has a `return_url`. |

## Behavior

### Starting and completing authorization

- Identity allows only the configured Google and GitHub endpoints and the exact registered callback URLs. A client cannot supply a provider URL or a provider callback URL. A client can supply a frontend return URL only from the configured list.
- Identity uses the authorization code flow with PKCE `S256` and a fresh one-time `state` value for both providers. It binds the state, the PKCE verifier, the provider, and the attempt token to 1 login attempt. Google also receives a fresh `nonce`, which Identity checks in its ID token.
- Identity sends the user to the selected provider, and the provider returns an authorization code to its fixed Identity callback. Identity rejects a missing, changed, expired, or reused state before it exchanges the code, and it rejects a callback on the wrong provider route.
- Identity exchanges the code on the server with the matching provider and the original PKCE verifier, and it checks the redirect URI of the attempt. Provider tokens stay on the server, and Identity discards them after it gets the identity proof.
- The callback records 1 checked provider result and creates a one-time handoff code. It creates no Flowspace account or session. A provider denial or a failed exchange records a failure and creates no handoff code. A repeated callback cannot replace a checked result.
- The callback checks its source limit before it looks up the attempt. A limit rejection, an unknown provider route, and an unknown or expired `state` show the callback page with a safe error and no handoff code, because Identity has no return URL for them.
- The attempt token and the handoff code each have at least 128 random bits, and Identity stores only keyed verifiers of both. The attempt, its callback, and its handoff code expire 10 minutes after the start. Each code and each attempt succeeds once.

### API client handoff

- A `StartProviderLogin` request without `return_url` returns the attempt token in the `attempt_token` field and sets no cookie.
- After a successful callback, the callback page shows the handoff code. After a failed callback, it shows a safe error. The callback page has no third-party resources and no scripts. It has the `Cache-Control: no-store` and `Referrer-Policy: no-referrer` headers and a `Content-Security-Policy` header that allows no sources. The API client copies the code and calls `CreateProviderSession` with the attempt token and the handoff code.
- A browser page cannot claim a session without the attempt token, and an API client cannot claim one without the handoff code.

### Browser redirect handoff

- A `StartProviderLogin` request with `return_url` must pass the `Origin` and content type checks of the Identity password login and sessions specification. Identity compares `return_url` with each entry of `PROVIDER_RETURN_URLS` byte for byte, without URL normalization, and returns `InvalidArgument` when no entry matches.
- Identity stores the return URL with the attempt, leaves the `attempt_token` field empty, and sets the `__Host-flowspace-login-attempt` cookie. The cookie has the attributes of the refresh-token cookie, and its `Max-Age` ends it at the attempt expiry.
- After a successful callback, Identity answers with HTTP 303 to the stored return URL with the fragment `#handoff_code=<code>`. After a failed callback, the fragment is `#error=` and a callback error code, with no handoff code. The response has the `Cache-Control: no-store` and `Referrer-Policy: no-referrer` headers. The callback does not change the attempt cookie.
- The browser sends no fragment to a server or in a `Referer` header. Each page in `PROVIDER_RETURN_URLS` answers directly, without a server redirect, because the browser copies the fragment to a redirect target. The frontend reads the fragment and removes it from the address bar before it loads a third-party script or follows a redirect.
- When a `CreateProviderSession` request has an empty `attempt_token` field, Identity reads the attempt token from the login attempt cookie. Such a request must pass the `Origin` and content type checks, and it always uses cookie delivery for the refresh token, regardless of the value of `use_refresh_token_cookie`. A request with an empty `attempt_token` field and no cookie returns `Unauthenticated`.
- Identity reads the login attempt cookie only in `CreateProviderSession`. It sends an expired login attempt cookie after a successful completion and after a failure that ends the attempt, such as an unknown, expired, used, or failed attempt, the fifth wrong handoff code, or an account error. A failure that leaves the attempt usable, such as a full limit or an unavailable dependency, keeps the cookie. An API client ignores the expired cookie.
- A second login attempt in the same browser, such as in another tab, replaces the cookie of the first attempt, and the first attempt can no longer complete there.
- A handoff code works only in the browser that holds the matching attempt cookie, so a crafted redirect link cannot sign a victim in to the account of an attacker.
- The frontend includes credentials in a start with `return_url` and in a completion that reads the attempt cookie.
- A frontend on another site, such as a Vercel preview domain, gets no attempt cookie and cannot use the browser redirect handoff.

### Provider identity and email

- For Google, Identity checks the ID token signature against the published Google keys. It checks the issuer, the audience, the expiry, the nonce, and a nonempty `sub`. It uses `sub` as the provider identity, never the email address. A missing or false `email_verified` value gives no verified-email proof.
- For GitHub, Identity calls the authenticated user endpoint after it exchanges the code, and it uses the stable numeric user `id` as the provider identity. It requests only the scope that it needs to read email addresses. When the provider subject is new, Identity checks the primary verified email through the authenticated email endpoint. A profile email alone is not verified-email proof.
- Before it creates a new account, Identity checks the provider email with the comparison rules of the [Identity signup and email verification](identity-signup-and-email-verification.md) specification. It never trusts an email, a provider subject, or a verified state from the client.
- A known pair of provider name and provider subject identifies its Flowspace account even when the provider email changes, loses verification, or is absent. Provider login does not change the stored Flowspace email or its verified state. When the GitHub email endpoint fails, a login for an existing link still succeeds after the authenticated user endpoint proves the subject.
- A new provider identity needs a usable verified email from the provider. Google must supply an email with `email_verified=true`, and GitHub must supply a primary email with `verified=true` through the authenticated email endpoint. Without this proof or the email, Identity creates no account, link, or session.
- When the email of a new provider identity is not in use, Identity creates 1 provider-only account with a new stable subject, the email, and a unique provider link, in 1 transaction. The account has no password. A verified Google `@gmail.com` address, or a Google Workspace address with an `hd` claim, starts with `email_verified=true`. Other Google addresses and all GitHub addresses start with `email_verified=false`, as the [Google ID token](https://developers.google.com/identity/gsi/web/guides/verify-google-id-token) guide explains.
- For a new account that starts unverified, Identity creates a 6-digit email challenge and a durable delivery request in the account transaction. The user gets a session but cannot use Workspace until `VerifyEmail` succeeds. The challenge uses the limits of the signup specification. When delivery fails after the commit, Identity retries it.
- When another Flowspace account owns the email, Identity creates no account, link, or session. The user signs in through an existing method and uses provider link. Matching email addresses never link accounts.

### Session, failure, and abuse handling

- `CreateProviderSession` consumes the handoff proof and creates 1 session in 1 transaction. Concurrent completions create at most 1 account, link, and session. The session uses the lifetime rules of the Identity password login and sessions specification.
- An unverified account cannot use Workspace even with a session. Live session checks return the current verified state, and Workspace still owns membership and roles.
- When a `StartProviderLogin` response is lost, a retry creates a new attempt, and the old attempt expires after 10 minutes. When a `CreateProviderSession` response is lost, the user starts a new provider login. The first session can stay active until it expires or the user logs out. Identity never stores a refresh token in a form that it can replay.
- Identity allows at most 60 `StartProviderLogin` requests and 120 callbacks for each trusted source address in an hour. The callback limit counts successful and invalid requests. Identity ends 1 attempt after 5 failed handoff claims. Identity also allows at most 100 failed `CreateProviderSession` requests for each source address in an hour, including unknown attempt tokens.
- The limits count retries, use shared state across replicas, and fail closed when that state is unavailable. Only configured edge proxies can supply the source address.
- A timeout of a required provider call, a denied consent, an invalid proof, or a database failure leaves no partial account, link, or session. Identity reports success only after the account, link, and session transaction commits.

### Data and compatibility

- A migration adds a nullable return URL to provider attempts. Existing attempts have no return URL, so they keep the callback page.
- `return_url` and `use_refresh_token_cookie` are new optional fields, and the `attempt_token` field of `CreateProviderSessionRequest` stops being required. An API client that sends neither new field keeps its behavior.
- Older code ignores the return URL and the attempt cookie, so it shows the callback page for a browser attempt, and the browser cannot complete it. Before a rollback, the web frontend stops sending `return_url`. The pending browser attempts expire within 10 minutes, and the users start again.

### Diagnostics

Security events record the provider, the operation, and the outcome. Logs, traces, metrics, errors, and browser content other than the handoff page and the redirect fragment must never contain these values:

- Provider authorization codes and provider tokens.
- Attempt tokens, handoff codes, access tokens, and refresh tokens.
- The `Cookie` and `Set-Cookie` headers, and the `Location` header of a callback redirect.
- Full email addresses.

## Testing strategy

| Risk | Test level |
| --- | --- |
| A forged callback, an injected code, a provider mix-up, a wrong PKCE verifier, or a reused state completes an attempt (`ID-T06`) | Unit, Integration |
| An altered Google claim or GitHub response, or a missing email proof for a new identity, creates a session (`ID-T07`) | Unit |
| A matching email links an account, or an email collision and a missing email give different results (`ID-T01`, `ID-T08`) | Integration |
| A known provider subject cannot log in after its email changes or disappears (`ID-T07`) | Integration |
| A Google Gmail or Workspace account starts unverified, or another new account starts verified (`ID-T03`) | Unit |
| An expired, used, or failed handoff proof creates a session (`ID-T09`, `ID-T19`) | Unit, Integration |
| Concurrent completions create 2 accounts, links, or sessions (`ID-T12`) | Integration |
| A start, callback, handoff, or failure limit misses its boundary across replicas (`ID-T16`) | Integration |
| A provider outage or a database failure leaves a partial account, link, or session (`ID-T16`) | Integration |
| A malformed provider value, attempt token, handoff code, or return URL reaches the database (`ID-T15`) | Unit |
| A return URL that differs from a configured entry by any byte is accepted (`ID-T27`) | Unit |
| A callback redirect puts the handoff code outside the fragment, or a callback redirect or callback page lacks `no-store` or `no-referrer` (`ID-T27`) | Unit |
| A handoff code completes without its attempt cookie, or a cookie of another purpose completes a login (`ID-T27`) | Integration |
| A cookie request from another origin, with no `Origin`, or with a form content type starts or completes an attempt (`ID-T23`) | Unit |
| The attempt cookie has wrong attributes, stays after an ending failure, or is cleared after a failure that leaves the attempt usable (`ID-T24`) | Unit |
| A start with `return_url` returns the attempt token, or a cookie completion returns the refresh token, in the body (`ID-T26`) | Unit |
| Telemetry, errors, or the callback page contain a reusable secret or a full email (`ID-T17`) | Integration |
| The API client flow or the browser redirect flow fails end to end through the generated REST routes and the callback | Cluster |

Run live provider smoke tests only with disposable provider accounts.

## Implementation boundaries

Always do these actions:

- Identify a provider account by the provider name and the stable provider subject.
- Keep provider tokens on the server, and check email proof before a new account passes the verified-email gate.

Ask the maintainer before these actions:

- Change the API client handoff, the browser redirect handoff, or the callback error codes, or put a Flowspace token on a browser page or in a redirect.
- Change the provider email rules, the account-collision response, or the linking rules.
- Change the attempt, callback, or handoff limits.
- Accept a return URL by prefix, pattern, or normalized comparison.

Never do these actions:

- Do not link accounts by matching email addresses, or move a provider link between subjects during login.
- Do not accept provider profile data, a provider token from the client, or an unverified email as identity proof.
- Do not put a reusable secret in a URL, a browser page, a log, a trace, a metric, or a shared cache, or put a handoff code in a query string.

## Success criteria

1. Given an API client starts a Google or GitHub login without `return_url`, When Identity answers, Then it returns 1 provider authorization URL and a separate secret attempt token, and it sets no cookie.
2. Given a valid provider code and state for an attempt without a return URL, When the provider calls back, Then Identity checks the provider proof once and shows a one-time handoff code without Flowspace tokens.
3. Given a callback has a wrong provider, state, nonce, verifier, redirect URI, issuer, or audience, When Identity handles it, Then Identity rejects it without a handoff code, account, link, or session.
4. Given a first-time provider identity has an unused verified email, When the user completes provider login, Then Identity creates 1 provider-only account, 1 link, and 1 session for a new stable subject.
5. Given a new Google identity has a verified Gmail or Google Workspace email, When the user completes provider login, Then the new account starts verified and passes the Workspace verified-email gate.
6. Given a new Google identity with a verified third-party email or a new GitHub identity with a verified primary email, When the user completes provider login, Then the account gets a session and a 6-digit email challenge, and Workspace rejects it until Flowspace verifies the email.
7. Given a new provider-only account has a Flowspace email challenge, When the account completes it, Then the next live session check returns `email_verified=true`, so Workspace can apply its membership rules.
8. Given a provider identity already has a link but its email changed or is missing, When the user logs in again, Then Identity signs in the same subject and keeps the stored email, its verified state, and the Workspace access.
9. Given a verified provider email belongs to another Flowspace account, or a new provider identity has no usable verified email, When the user completes provider login, Then Identity creates no account, link, or session and returns the same safe status and response.
10. Given an invalid, expired, or used handoff proof, When a client submits it, Then Identity issues no session, and the error has no reusable secret.
11. Given 1 handoff proof, When 2 clients claim it at the same time, Then at most 1 account, link, and session transition commits.
12. Given a successful `StartProviderLogin` or `CreateProviderSession` response is lost, When the client starts a new provider login, Then Identity creates a new attempt and a new token pair for the same subject without a replay of the lost refresh token.
13. Given authorization has started, When a required provider call or Identity dependency fails, Then Identity leaves no partial account, link, or session and reports a safe temporary failure.
14. Given a provider-login limit is reached, When another request arrives, Then Identity rejects it without a new attempt, handoff code, or session.
15. Given a browser on an allowed origin starts a login with a `return_url` from `PROVIDER_RETURN_URLS`, When Identity answers, Then the `attempt_token` field is empty, and the response sets the `__Host-flowspace-login-attempt` cookie.
16. Given a `return_url` that differs from each configured entry by at least 1 byte, When a browser starts a login, Then Identity returns `InvalidArgument` and creates no attempt.
17. Given a successful callback for an attempt with a return URL, When Identity answers, Then it sends HTTP 303 to the return URL with `#handoff_code=<code>`, `Cache-Control: no-store`, and `Referrer-Policy: no-referrer`, and a failed callback sends `#error=` with a callback error code.
18. Given the browser that holds the login attempt cookie, When it calls `CreateProviderSession` with an empty `attempt_token` field and the handoff code, Then Identity issues a session with cookie delivery and sends an expired login attempt cookie.
19. Given a handoff code from a redirect, When another browser without the matching attempt cookie submits it, Then Identity issues no session.
