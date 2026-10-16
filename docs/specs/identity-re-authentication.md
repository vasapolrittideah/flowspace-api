# Spec: Identity re-authentication

Module ID: `identity-re-authentication`

Status: Approved

## Objective

Let a signed-in user prove their identity again before a sensitive action, such as a change to a credential, an email address, a provider link, or the account itself. A session alone shows only that someone signed in on that device, up to 90 days earlier. Re-authentication shows that the owner still controls the session now. The first users are API clients, and all data is disposable under [ADR-0022](../adr/0022-single-host-storage-holds-disposable-data.md).

The user can prove their identity with the account password, with a Google or GitHub account that is linked to the account, or with a 6-digit code sent to the verified account email. Each method gives the same result: the current session is re-authenticated for 10 minutes. Each provider-only account and each password account has at least 1 method.

This module gives only the mechanism and the error that a sensitive method returns. Each later specification that adds a sensitive method states that the method requires re-authentication.

## Scope and ADRs

The scope covers the re-authentication time and the first window of each session, and the password, email code, and provider methods. It also covers the limits of each method and the error that a sensitive method returns outside the window.

This capability excludes these items:

- The list of sensitive methods. Each later specification names its own sensitive methods.
- Multi-factor authentication (MFA) and a second factor for re-authentication.
- Different strengths for different methods, such as an action that accepts only the password.
- A one-time token for each sensitive action.
- The re-authentication state in `CheckSession` for other services, such as Workspace.
- An email notice after a successful re-authentication.

These decisions apply:

- [ADR-0031](../adr/0031-flowspace-owns-authentication-and-revocable-sessions.md): Identity owns sessions, passwords, provider links, and email codes, and it makes sure on the server that each session is valid.
- [ADR-0045](../adr/0045-provider-re-authentication-reuses-the-one-time-handoff.md): provider re-authentication reuses the callback and the one-time handoff of provider login for the current session, and `StartProviderReauthentication` rejects `Idempotency-Key`.

## Contract

Use package `flowspace.identity.v1` and service `IdentityService`.

The `provider` field uses the existing `Provider` enum of provider login and accepts only `PROVIDER_GOOGLE` and `PROVIDER_GITHUB`. `PROVIDER_UNSPECIFIED` and unknown values are malformed. The `code` field is a string of exactly 6 ASCII digits, and leading zeroes are significant, as in the [Identity signup and email verification](identity-signup-and-email-verification.md) specification.

### Methods

| RPC | Public HTTP route | Request fields | Response fields |
| --- | --- | --- | --- |
| `ReauthenticateWithPassword` | `POST /v1/password-reauthentications` | `password` (required) | `reauthentication_expires_at` |
| `RequestReauthenticationCode` | `POST /v1/reauthentication-codes` | None | `accepted` |
| `ReauthenticateWithCode` | `POST /v1/code-reauthentications` | `code` (required) | `reauthentication_expires_at` |
| `StartProviderReauthentication` | `POST /v1/provider-reauthentication-attempts` | `provider` (required) | `authorization_url`, `attempt_token`, `attempt_expires_at` |
| `CompleteProviderReauthentication` | `POST /v1/provider-reauthentications` | `attempt_token` (required), `handoff_code` (required) | `reauthentication_expires_at` |

### Method requirements

| RPC | Authentication | Retry |
| --- | --- | --- |
| `ReauthenticateWithPassword` | `Bearer access token` | `Safe to retry` |
| `RequestReauthenticationCode` | `Bearer access token` | `Safe to retry` |
| `ReauthenticateWithCode` | `Bearer access token` | `Safe to retry` |
| `StartProviderReauthentication` | `Bearer access token` | `Rejects Idempotency-Key` under [ADR-0045](../adr/0045-provider-re-authentication-reuses-the-one-time-handoff.md) |
| `CompleteProviderReauthentication` | `Bearer access token` | `Safe to retry` |

### Resource fields

Re-authentication result:

| Protobuf field | JSON field | Meaning |
| --- | --- | --- |
| `reauthentication_expires_at` | `reauthenticationExpiresAt` | The server time when the re-authentication window of the current session ends, output only |
| `accepted` | `accepted` | `true` when Identity accepted the code request, output only |
| `authorization_url` | `authorizationUrl` | The provider authorization URL that the user opens in a browser, output only |
| `attempt_token` | `attemptToken` | The secret token of the provider attempt, at least 128 random bits, output only |
| `attempt_expires_at` | `attemptExpiresAt` | The server time when the provider attempt expires, output only |

### Published events

| Event | Topic | Fields |
| --- | --- | --- |
| `EmailDeliveryRequested` | `identity.email-delivery.v1` | `event_id`, `challenge_id`, `purpose` |

### Errors

| Condition | gRPC status | HTTP status |
| --- | --- | --- |
| Malformed request | `InvalidArgument` | 400 |
| Wrong password, or the account has no password | `InvalidArgument` | 400 |
| Wrong, expired, or used code | `InvalidArgument` | 400 |
| Missing, invalid, or inactive session | `Unauthenticated` | 401 |
| The account email is not verified, so Identity cannot send a code | `FailedPrecondition` | 400 |
| The provider attempt is unknown, expired, used, or failed, or the provider account is not linked to the subject | `FailedPrecondition` | 400 |
| A client calls a sensitive method outside the re-authentication window | `PermissionDenied` | 403 |
| A limit is reached | `ResourceExhausted` | 429 |
| Identity, its limit store, or the provider is unavailable | `Unavailable` | 503 |

## Behavior

### Re-authentication window

- Identity stores 1 re-authentication time on each session. The window of a session is open from that time until 10 minutes later. At the exact end time, the window is closed. Identity uses only server time.
- `CreateAccount`, `ClaimUnverifiedAccount`, `CreatePasswordSession`, and `CreateProviderSession` set the re-authentication time of the new session to its creation time, because the login is a new proof.
- When the window is closed, a successful re-authentication sets the time of the current session to the commit time and returns the new end time. When the window is open, a successful re-authentication keeps the current time and returns the current end time, so a retry has no additional effect. After the window closes, the user re-authenticates again to open a new 10-minute window.
- `RefreshSession` does not change the re-authentication time. The window never extends past the idle or absolute expiry of the session, and it ends when the session is revoked or logged out.
- The window belongs to 1 session. A re-authentication in 1 session does not open the window of another session of the same subject.
- The 3 methods give the same window. A sensitive method does not ask which method opened it.

### Sensitive methods

- A later specification marks each of its methods that changes a credential, an email address, a provider link, the sessions of other devices, or the account itself as a sensitive method.
- A sensitive method makes sure that the session of the access token has an open window in the same transaction as its change. If the window is closed at that time, the method changes nothing and returns `PermissionDenied` with an `ErrorInfo` detail whose reason is `REAUTHENTICATION_REQUIRED` and whose domain is `identity.flowspace`.
- The client then re-authenticates and calls the sensitive method again. A sensitive method that started inside the window and commits after the window ends fails with the same error.

### Password method

- `ReauthenticateWithPassword` uses the Argon2id hash of the subject of the access token to make sure that the password is correct, with the same Unicode NFC normalization as password login.
- A wrong password and an account without a password return the same `InvalidArgument` error and do not change the window.
- A retry after a lost response makes sure that the password is correct again. It counts against the limits again, and it returns the same end time while the window is open.

### Email code method

- `RequestReauthenticationCode` creates a 6-digit challenge for the subject, its current verified email, and the re-authentication purpose, and a durable delivery request in the same transaction. The `purpose` field of `EmailDeliveryRequested` is `reauthenticate`, next to `verify-email`, `claim-account`, and `password-reset`.
- Only an account whose current email is verified can request a code. Other accounts receive `FailedPrecondition` and no code.
- A re-authentication code cannot claim an account or reset a password, and it never changes the verified state. `VerifyEmail` keeps its behavior for a verified account: it succeeds without a change. A verification, claim, or reset code cannot re-authenticate a session.
- The challenge belongs to the subject, not to 1 session. Any active session of the subject can use the code, and the code opens the window of that session only.
- A new code request replaces only the previous re-authentication challenge. A request within 60 seconds of the previous request returns `accepted=true` and does not replace the challenge.
- The code expires 10 minutes after Identity issues it and succeeds once. A retry with a used code returns the code error, and the window that the first request opened stays open.

### Provider method

- `StartProviderReauthentication` creates a provider attempt that is bound to the subject and the session of the access token and has the re-authentication purpose. It uses the provider protocol, the callback route, the callback page, and the 10-minute attempt expiry of provider login.
- The callback route reads the purpose from the attempt. A provider result for 1 purpose cannot complete an attempt of the other purpose.
- `CompleteProviderReauthentication` succeeds only from the session that started the attempt, with the matching attempt token and handoff code. The provider name and provider subject must match an existing provider link of the subject.
- The provider email and its verified state do not matter. Identity changes no account, link, email, or session other than the re-authentication time of the current session, and it issues no token.
- A provider subject that is not linked to the subject, or that is linked to another subject, returns `FailedPrecondition` and changes no link.
- If the start response is lost, the client starts a new attempt and the old attempt expires. If the completion response is lost, the window is open. A retry returns `FailedPrecondition` because the handoff code is used.

### Security and abuse

- Each method first makes sure that the access token is valid and the session is active, as `CheckSession` does.
- Each method uses the same limit buckets as the existing method of its kind, not new buckets.
- `ReauthenticateWithPassword` counts as 1 attempt against the source and email limits of [Password login](identity-password-login-and-sessions.md#password-login). The email limit uses the current account email. A full limit returns `ResourceExhausted` before password hashing.
- `RequestReauthenticationCode` and `ReauthenticateWithCode` count against the code-request and wrong-guess limits that [Email code and verification](identity-signup-and-email-verification.md#email-code-and-verification) and [Abuse and failure handling](identity-signup-and-email-verification.md#abuse-and-failure-handling) share across verification, claim, and reset codes.
- `StartProviderReauthentication`, the callback, and `CompleteProviderReauthentication` count against the limits of [Session, failure, and abuse handling](identity-provider-login.md#session-failure-and-abuse-handling) for `StartProviderLogin`, the callback, and `CreateProviderSession`.
- All limits use shared state across replicas, count retries, and fail closed with `Unavailable` when the limit store is unavailable.
- A dependency failure, a timeout, or a cancellation does not open the window. Identity returns a result only after the change of the re-authentication time commits.
- A failed re-authentication does not revoke the session or lock the account. Only the limits slow repeated failures.
- A person who has both an active session and the mailbox of the account can re-authenticate with an email code. The maintainer accepted this risk: the 3 methods have the same strength.

### Data and compatibility

- A migration adds a nullable re-authentication time to sessions, a re-authentication purpose for email challenges, and a purpose, subject, and session ID to provider attempts. Existing sessions have no re-authentication time, so their windows are closed. Existing provider attempts get the login purpose.
- Older code ignores the new columns. Before a rollback to older code, delete the pending provider attempts with the re-authentication purpose, so that older code cannot complete them as provider login.
- No sensitive method exists before a later specification adds one, so this module changes the behavior of no existing method.

### Diagnostics

Logs, traces, metrics, errors, and security events record the method and the outcome of each re-authentication. They must never contain these values:

- Passwords, codes, attempt tokens, handoff codes, provider authorization codes, and provider tokens.
- Access tokens, refresh tokens, and full email addresses.
- Provider subjects, session IDs, and subject IDs in metric labels.

## Testing strategy

| Risk | Test level |
| --- | --- |
| A new session from signup, account claim, password login, or provider login starts without an open window (`ID-T09`) | Unit, Integration |
| A window stays open at or after its 10-minute end, a refresh extends it, or a retry moves its end time (`ID-T19`) | Unit |
| The window guard admits a change outside the window, or a change commits after the window ends in a race with its check, proven through a test-only sensitive method (`ID-T12`) | Integration |
| A re-authentication opens the window of another session (`ID-T09`) | Integration |
| A logout, an all-session logout, or a password reset leaves a window usable (`ID-T13`) | Integration |
| A wrong password and an account without a password give different results (`ID-T01`) | Unit |
| Password re-authentication bypasses or does not count against the password-login limits (`ID-T02`, `ID-T16`) | Integration |
| An account with an unverified email receives a re-authentication code (`ID-T03`) | Unit |
| A code of another purpose re-authenticates, or a re-authentication code claims, resets, or changes the verified state (`ID-T03`) | Unit, Integration |
| Code requests or guesses bypass the shared code limits across replicas (`ID-T03`, `ID-T16`) | Integration |
| A provider result for login completes re-authentication, or the reverse (`ID-T06`) | Integration |
| A provider account that is not linked, or that is linked to another subject, re-authenticates (`ID-T07`, `ID-T08`) | Integration |
| A provider attempt completes from another session (`ID-T06`, `ID-T09`) | Integration |
| Provider re-authentication bypasses the shared provider-login limits across replicas (`ID-T16`) | Integration |
| A concurrent use of 1 code or handoff opens 2 windows or changes a link (`ID-T12`) | Integration |
| A failed re-authentication revokes the session or locks the account (`ID-T02`) | Unit |
| A malformed provider value, code, or request body reaches the methods (`ID-T15`) | Unit |
| An unavailable limit store, database, or provider, a timeout, or a cancellation opens the window (`ID-T16`) | Integration |
| Telemetry or errors contain a password, code, token, or full email (`ID-T17`) | Integration |
| The flow fails end to end through the generated REST routes, Mailpit, and the callback page | Cluster |

## Implementation boundaries

Ask the maintainer before these actions:

- Change the 10-minute window, the 3 methods, or the rule that a new session starts with an open window.
- Add the re-authentication state to `CheckSession`.
- Give a method a different strength, or let a sensitive method accept only some methods.

Never do these actions:

- Do not add a public sensitive method in this module. Test the window guard through a test-only method.

## Success criteria

1. Given a user just logged in with a password, a provider, signup, or an account claim, When the session calls a sensitive method within 10 minutes, Then the method does not return the re-authentication error.
2. Given a session started more than 10 minutes ago without any later re-authentication, When it calls a sensitive method, Then Identity changes nothing and returns `PermissionDenied` with the reason `REAUTHENTICATION_REQUIRED`.
3. Given a session refreshes after its window closes, When it calls a sensitive method with the new access token, Then Identity returns the re-authentication error.
4. Given an active session of a password account has a closed window and at least 10 minutes before its idle and absolute expiry, When the session calls `ReauthenticateWithPassword` with the correct password, Then the response has an end time 10 minutes after the commit, and a sensitive method succeeds until that time.
5. Given a session has an open window, When it re-authenticates again with any method, Then the response returns the same end time, and the window does not move.
6. Given a wrong password or an account without a password, When the session calls `ReauthenticateWithPassword`, Then Identity returns the same `InvalidArgument` error, and the window stays closed.
7. Given an account with a verified email, When the session requests a re-authentication code and submits it, Then the window opens, and the code cannot be used again.
8. Given an account with an unverified email, When the session calls `RequestReauthenticationCode`, Then Identity returns `FailedPrecondition` and sends no code.
9. Given a verification, claim, or reset code, When the session submits it to `ReauthenticateWithCode`, Then Identity returns the code error. A re-authentication code fails in `ClaimUnverifiedAccount` and `ResetPassword` and never changes the verified state.
10. Given a provider-only account with a linked Google or GitHub account, When the session starts provider re-authentication, completes the provider callback, and submits the handoff code, Then the window of that session opens, and no token, session, or link is created.
11. Given a provider account that is not linked to the subject, When the session completes provider re-authentication, Then Identity returns `FailedPrecondition`, and the window stays closed.
12. Given a provider re-authentication attempt from 1 session, When another session or `CreateProviderSession` submits its attempt token and handoff code, Then Identity rejects it without a window, session, or token.
13. Given a subject has 2 sessions, When 1 session re-authenticates, Then the other session still receives the re-authentication error.
14. Given a session has an open window, When the session logs out or a password reset revokes it, Then its access token cannot call a sensitive method.
15. Given a re-authentication limit is full, When the session tries again, Then Identity returns `ResourceExhausted` without password hashing, a code, or a provider attempt.
16. Given repeated failed re-authentication attempts, When the limits are reached, Then the session stays active, and the account can still log in after the limit window.
17. Given the re-authentication tests run, When a test inspects logs, traces, metrics, and errors, Then they contain no password, code, token, handoff code, or full email address.
