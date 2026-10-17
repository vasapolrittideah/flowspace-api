# Spec: Identity provider link

Module ID: `identity-provider-link`

Status: Approved

## Objective

Let a signed-in user link a Google or GitHub account to their Flowspace account, so that the provider account signs in to it and re-authenticates it. The user proves control of the provider account through the provider flow, and Identity links the account that the provider proved, never an account with a matching email address. The first users are API clients, and all data is disposable under [ADR-0022](../adr/0022-single-host-storage-holds-disposable-data.md).

An account links at most 1 account of each provider, and a provider account links to at most 1 Flowspace account. Identity never moves a link between Flowspace accounts.

## Scope and ADRs

The scope covers the start and the completion of a provider link for the current session, the callback purpose for a link, the link conflicts, and the limits.

This capability excludes these items:

- A move of a link from 1 Flowspace account to another.
- A change to the email address or the verified-email state of the account from the provider email.
- A method that lists the links of an account.
- An email notice after a link.
- Providers other than Google and GitHub.

These decisions apply:

- [ADR-0013](../adr/0013-acting-identity-comes-from-the-token.md): the account comes from the access token, never from the request.
- [ADR-0031](../adr/0031-flowspace-owns-authentication-and-revocable-sessions.md): a signed-in user links a provider explicitly, and matching email addresses never link accounts.
- [ADR-0034](../adr/0034-provider-login-uses-a-one-time-handoff.md): the provider callback, the handoff page, and the attempt rules come from provider login.
- [ADR-0046](../adr/0046-provider-link-reuses-the-one-time-handoff.md): provider link reuses the callback and the one-time handoff for the current session, and `StartProviderLink` rejects `Idempotency-Key`.

## Contract

Use package `flowspace.identity.v1` and service `IdentityService`.

The `provider` field uses the existing `Provider` enum of provider login and accepts only `PROVIDER_GOOGLE` and `PROVIDER_GITHUB`. `PROVIDER_UNSPECIFIED` and unknown values are malformed.

### Methods

| RPC | Public HTTP route | Request fields | Response fields |
| --- | --- | --- | --- |
| `StartProviderLink` | `POST /v1/provider-link-attempts` | `provider` (required) | `authorization_url`, `attempt_token`, `attempt_expires_at` |
| `CompleteProviderLink` | `POST /v1/provider-links` | `attempt_token` (required), `handoff_code` (required) | None |

### Method requirements

| RPC | Authentication | Retry |
| --- | --- | --- |
| `StartProviderLink` | Bearer access token | Rejects `Idempotency-Key` under [ADR-0046](../adr/0046-provider-link-reuses-the-one-time-handoff.md) |
| `CompleteProviderLink` | Bearer access token | Safe to retry |

### Resource fields

Link attempt:

| Protobuf field | JSON field | Meaning |
| --- | --- | --- |
| `authorization_url` | `authorizationUrl` | The provider authorization URL that the user opens in a browser, output only |
| `attempt_token` | `attemptToken` | The secret token of the attempt, at least 128 random bits, output only |
| `attempt_expires_at` | `attemptExpiresAt` | The server time when the attempt expires, 10 minutes after its start, output only |

### Errors

| Condition | gRPC status | HTTP status |
| --- | --- | --- |
| Malformed request or provider value | `InvalidArgument` | 400 |
| Missing, invalid, or inactive session | `Unauthenticated` | 401 |
| The attempt is unknown, expired, failed, of another purpose, or of another session, or the handoff code is wrong | `FailedPrecondition` | 400 |
| The account already links another account of the provider | `FailedPrecondition` | 400 |
| A client calls `StartProviderLink` or `CompleteProviderLink` outside the re-authentication window | `PermissionDenied` | 403 |
| The provider account is linked to another Flowspace account | `AlreadyExists` | 409 |
| A limit is reached | `ResourceExhausted` | 429 |
| Identity, its database, its limit store, or the provider is unavailable | `Unavailable` | 503 |

The 2 `FailedPrecondition` conditions use 2 different safe messages. The account-conflict message tells the user to unlink the current account of the provider first.

## Behavior

### Starting a link

- `StartProviderLink` and `CompleteProviderLink` are sensitive methods under the [Identity re-authentication](identity-re-authentication.md) specification. `StartProviderLink` makes sure that the session of the access token has an open re-authentication window before it creates the attempt, so that the user does not visit the provider for nothing.
- `StartProviderLink` creates an attempt with the link purpose that is bound to the subject and the session of the access token. It uses the provider protocol, the callback route, the callback page, and the 10-minute attempt expiry of the [Identity provider login](identity-provider-login.md) specification.
- For Google, the link attempt asks only for the `openid` scope. For GitHub, it asks for no extra scope, and Identity reads only the stable numeric user `id` from the authenticated user endpoint.
- If the start response is lost, the client starts a new attempt and the old attempt expires.

### Completing a link

- The callback route reads the purpose from the attempt. A provider result for login or re-authentication cannot complete a link attempt, and a link result cannot complete another purpose.
- `CompleteProviderLink` succeeds only from the session that started the attempt, with the matching attempt token and handoff code.
- `CompleteProviderLink` makes sure that the re-authentication window is open in the same transaction as the link. A window that closes while the user is at the provider makes the completion fail. The user re-authenticates and retries the same attempt before it expires, or starts a new attempt.
- Identity links the provider name and the provider subject that the callback proved. The proof of the provider account is not a re-authentication of the session.
- The provider email, its verified state, and its match with the account email do not matter. The email address and the verified-email state of the account do not change.
- When the provider subject is linked to another Flowspace account, Identity returns `AlreadyExists` and changes no link. The error does not name or describe the other account.
- When the account already links another provider subject of the same provider, Identity returns `FailedPrecondition` and changes no link. The user unlinks that account first through the [Identity provider unlink](identity-provider-unlink.md) specification.
- When the account already links the same provider subject, the completion succeeds without a change.
- Until the attempt expires, a retry of a completed attempt from the same active session, with the same attempt token and handoff code and an open re-authentication window, succeeds without a change. After the attempt expires, the retry returns `FailedPrecondition`.
- A conflict, an expired attempt, a wrong purpose, another session, or an exhausted handoff ends the attempt. A retry returns the same error.
- A closed re-authentication window, a full limit, an unavailable dependency, or a cancellation leaves the attempt unchanged. The client can fix the cause, such as a new re-authentication, and retry until the attempt expires.

### After a link

- The linked provider account signs in to the account through provider login, and it re-authenticates the account through provider re-authentication.
- The link issues no token, creates and revokes no session, and does not change the re-authentication window of the current session.
- Identity sends no notice after a link.

### Security and abuse

- `StartProviderLink`, the callback, and `CompleteProviderLink` count against the limits of [Session, failure, and abuse handling](identity-provider-login.md#session-failure-and-abuse-handling) for `StartProviderLogin`, the callback, and `CreateProviderSession`, not against new buckets. The limits use shared state across replicas, count retries, and fail closed with `Unavailable` when the limit store is unavailable.
- Identity serializes `CompleteProviderLink`, `UnlinkProvider`, and `SetPassword` for the same subject. The unique rules on links decide concurrent completions: at most 1 Flowspace account gets a provider subject, and at most 1 link of a provider exists on an account.
- A provider failure, a timeout, or a cancellation creates no link.

### Data and compatibility

- A migration adds the link purpose to provider attempts. The unique rules come from the provider login primary key and from the migration of the `identity-provider-unlink` module.
- The link and the consumption of the attempt commit in 1 transaction. A database failure before the commit leaves both unchanged.
- Older code ignores the new purpose. Before a rollback to the version before `StartProviderLink`, delete the pending attempts with the link purpose, so that older code cannot complete them as provider login. The links that exist stay in place.

### Diagnostics

Logs, traces, metrics, errors, and security events record the outcome of `StartProviderLink` and `CompleteProviderLink`. They must never contain these values:

- Provider subjects, provider authorization codes, and provider tokens.
- Attempt tokens, handoff codes, access tokens, and refresh tokens.
- Full email addresses.
- Session IDs and subject IDs in metric labels.

## Testing strategy

| Risk | Test level |
| --- | --- |
| A link starts or completes outside the re-authentication window, or the provider proof opens the window (`ID-T09`) | Integration |
| A provider result of login or re-authentication completes a link, or the reverse (`ID-T06`) | Integration |
| A link attempt completes from another session (`ID-T06`, `ID-T09`) | Integration |
| A link callback or completion succeeds at or after the attempt expiry, or a completed-attempt retry succeeds after it, before, at, and after the boundary (`ID-T19`) | Unit, Integration |
| A transient failure, such as a closed window or an unavailable dependency, ends the attempt, or a terminal failure leaves it usable | Integration |
| A forged or altered provider response creates a link (`ID-T07`) | Integration |
| A matching email links an account, or the provider email changes the account email state (`ID-T08`) | Integration |
| A provider subject of another account moves to this account, or the error names the other account (`ID-T01`, `ID-T08`) | Integration |
| An account gets a second link of 1 provider (`ID-T08`) | Integration |
| A repeated link of the same subject, or a retry of a completed attempt, gives an error or a second link | Integration |
| Concurrent completions give 2 accounts the same provider subject, or 2 links of 1 provider to 1 account (`ID-T12`) | Integration |
| A link changes a session, issues a token, or changes the re-authentication window | Integration |
| A new link does not sign in or re-authenticate the account | Integration |
| Link requests bypass the shared provider limits across replicas (`ID-T16`) | Integration |
| A provider failure, a timeout, or a cancellation leaves a link or a consumed attempt (`ID-T12`, `ID-T16`) | Integration |
| A malformed provider value, attempt token, or handoff code reaches the database (`ID-T15`) | Unit |
| Telemetry or errors contain a provider subject, a token, a code, or a full email (`ID-T17`) | Integration |
| The flow fails end to end through the generated REST routes, step-up, and the callback page | Cluster |

## Implementation boundaries

Ask the maintainer before these actions:

- Request a provider scope beyond `openid` for Google or beyond the default for GitHub.

## Success criteria

1. Given an account without a Google link and an open re-authentication window, When the session starts and completes a Google link, Then the Google account signs in to the account through provider login.
2. Given a closed re-authentication window, When the session calls `StartProviderLink`, Then Identity creates no attempt and returns `PermissionDenied` with the reason `REAUTHENTICATION_REQUIRED`.
3. Given a link attempt and a re-authentication window that closes before completion, When the session calls `CompleteProviderLink`, Then Identity creates no link and returns `PermissionDenied`.
4. Given a Google account linked to another Flowspace account, When the session completes a link with it, Then Identity returns `AlreadyExists`, keeps the other link, and creates no link.
5. Given an account linked to 1 GitHub account, When the session completes a link with another GitHub account, Then Identity returns `FailedPrecondition` with the unlink message and keeps the current link.
6. Given an account linked to a Google account, When the session completes a link with the same Google account, Then the completion succeeds and changes nothing.
7. Given a Google account whose email differs from the account email or is not verified, When the session completes a link, Then the link succeeds, and the account email and its verified state do not change.
8. Given a link attempt of 1 session, When another session or `CreateProviderSession` submits its attempt token and handoff code, Then Identity rejects it without a link, session, or token.
9. Given a provider result of a login or re-authentication attempt, When a client uses it to complete a link, Then Identity rejects it.
10. Given a completed link, When the session uses the tokens that it had before the link, Then the access token passes `CheckSession`, the refresh token rotates into a new token pair, and the re-authentication window keeps its end time.
11. Given a completed link, a lost response, an attempt that has not expired, and an open re-authentication window, When the same active session retries the completion with the same attempt token and handoff code, Then Identity returns success and creates no second link.
12. Given 2 accounts complete links to the same provider subject at the same time, When both commit, Then exactly 1 account has the link, and the other receives `AlreadyExists`.
13. Given the provider link limits are full, When the session starts or completes a link, Then Identity returns `ResourceExhausted` without an attempt or a link.
14. Given a linked GitHub account, When the session re-authenticates with GitHub, Then the re-authentication window opens.
15. Given the link tests run, When a test inspects logs, traces, metrics, and errors, Then they contain no provider subject, token, code, or full email address.
