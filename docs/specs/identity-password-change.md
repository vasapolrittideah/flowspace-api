# Spec: Identity password change

Module ID: `identity-password-change`

Status: Draft

## Objective

Let a signed-in user set a new password for their account without the email reset flow. A user with a password changes it, and a user of a provider-only account sets a first password, so that the account has a second way to sign in. The first users are API clients, and all data is disposable under [ADR-0022](../adr/0022-single-host-storage-holds-disposable-data.md).

After the change, every other session of the account ends, so a session that someone took with the old password stops. The session that made the change stays signed in.

## Scope and ADRs

The scope covers 1 method that sets the password of the caller, the revocation of the other sessions, and the password-change notice.

This capability excludes these items:

- A check of the current password. Re-authentication is the only proof.
- A password history, or a rule that rejects the current password as the new one.
- The removal of a password from an account.
- Multi-factor authentication (MFA).
- A change to `ResetPassword`.

These decisions apply:

- [ADR-0013](../adr/0013-acting-identity-comes-from-the-token.md): the account comes from the access token, never from the request.
- [ADR-0031](../adr/0031-flowspace-owns-authentication-and-revocable-sessions.md): Identity owns passwords and sessions, and a revocation is durable before success.
- [ADR-0035](../adr/0035-identity-signup-security-and-mail-delivery.md): a password change uses the same compromised-password check as signup, and it rejects the password when the check fails or times out.

## Contract

Use package `flowspace.identity.v1` and service `IdentityService`.

### Methods

| RPC | Public HTTP route | Request fields | Response fields |
| --- | --- | --- | --- |
| `SetPassword` | `POST /v1/password-changes` | `new_password` (required) | None |

### Method requirements

| RPC | Authentication | Retry |
| --- | --- | --- |
| `SetPassword` | Bearer access token | Do not retry |

### Errors

| Condition | gRPC status | HTTP status |
| --- | --- | --- |
| Malformed request, or a new password outside the password policy | `InvalidArgument` | 400 |
| Missing, invalid, or inactive session | `Unauthenticated` | 401 |
| A client calls `SetPassword` outside the re-authentication window | `PermissionDenied` | 403 |
| Identity cannot start password hashing before the request deadline, or Identity, its database, or the compromised-password check is unavailable | `Unavailable` | 503 |

## Behavior

### Setting the password

- `SetPassword` is a sensitive method under the [Identity re-authentication](identity-re-authentication.md) specification. It makes sure that the session of the access token has an open re-authentication window in the same transaction as the change. It does not ask for the current password.
- The new password must meet the normalization, length, blocklist, compromised-password, and Argon2id rules of the [Identity signup and email verification](identity-signup-and-email-verification.md) specification. Identity validates the request shape and the window before it checks or hashes the password. If the compromised-password check fails or times out, Identity returns `Unavailable` and changes nothing.
- For an account with a password, Identity replaces the password hash. For a provider-only account, Identity creates the password credential. The stable subject, the email address, the verified-email state, and the provider links do not change.
- Identity accepts a new password that is the same as the current one, and it treats the request as a change.
- After the change commits, the new password works through `CreatePasswordSession` and `ReauthenticateWithPassword`. A previous password whose normalized value differs from the new password fails.

### Sessions

- In the same transaction as the password change, Identity revokes every other session of the subject and invalidates its refresh tokens. It deletes the device data of those sessions, as the [Identity session management](identity-session-management.md) specification states.
- The session of the access token stays active. Its refresh token, its expiry times, its re-authentication time, and its device data do not change, and `SetPassword` issues no token.
- Identity serializes `SetPassword` with password login, session creation, and refresh for the same subject. A session that commits before the change is revoked, unless it is the current session. A login that starts after the commit must use the new password.
- A `CheckSession` or `RefreshSession` call that starts after the commit rejects each revoked session. A request that was admitted before the commit can finish.

### Notice

- When the current email of the account is verified, Identity queues a durable password-change notice to that address in the same transaction. The notice is the same notice as after a password reset, and it has no password.
- When the email is not verified, Identity changes the password and queues no notice.
- If delivery fails after the commit, Identity retries the notice without a change to the password or the sessions.

### Retry

- A retry after a lost response is a new password change. It revokes each session that the user created after the first change, and it queues a second notice when the email is verified. For this reason, the client does not retry automatically.
- The client uses `ListSessions` to inspect session revocation. The absence of other sessions alone does not confirm that the password changed, because those sessions can end for other reasons.

### Security and abuse

- `SetPassword` shares the bound on concurrent password-hash work of password login, so a burst of requests cannot exhaust CPU or memory. A request that cannot start hashing before its deadline returns `Unavailable` and changes nothing.
- `SetPassword` adds no limit bucket. Re-authentication and its limits already protect the method.
- A person who has an active session and either the account mailbox or the linked provider account can re-authenticate and set a new password without the old one. The maintainer accepted this risk.

### Data and compatibility

- The change needs no new table. Identity uses the existing password credential and the existing password-change notice queue.
- The password change, the revocations, the deletion of device data, and the notice commit in 1 transaction. A database failure, a timeout, or a cancellation before the commit leaves all of them unchanged.
- A rollback to the version before `SetPassword` removes the method and leaves each changed password in place.

### Diagnostics

Logs, traces, metrics, errors, and security events record the outcome of `SetPassword`. They must never contain these values:

- The new password, the old password, or a password hash.
- Access tokens, refresh tokens, and full email addresses.
- Session IDs and subject IDs in metric labels.

## Testing strategy

| Risk | Test level |
| --- | --- |
| `SetPassword` changes a password outside the re-authentication window (`ID-T09`) | Integration |
| A failed or timed-out compromised-password check accepts the password (`ID-T02`) | Unit, Integration |
| A new password outside the password policy is accepted, or Identity hashes before it checks the window (`ID-T02`, `ID-T16`) | Unit |
| A different old password still logs in or re-authenticates after the change, or a new password equal to the old one is rejected (`ID-T02`) | Integration |
| A provider-only account gets no password credential, or loses a provider link or its email state | Integration |
| Another session still refreshes or passes `CheckSession` after the commit (`ID-T13`, `ID-T14`) | Integration |
| A revoked session keeps its device data (`ID-T21`) | Integration |
| The current session loses its refresh token, its window, or its device data | Integration |
| A login or refresh races the change and keeps an old-password session (`ID-T12`) | Integration |
| A database failure, a timeout, or a cancellation leaves part of the change committed (`ID-T12`, `ID-T16`) | Integration |
| A repeated request after a lost response gives an effect other than a second change and, for a verified email, a second notice | Integration |
| A notice is missing for a verified email, is queued for an unverified email, or contains a password (`ID-T04`, `ID-T17`) | Integration |
| A burst of requests exhausts the password-hash bound (`ID-T16`) | Integration |
| A malformed request or password value reaches hashing (`ID-T15`) | Unit |
| Telemetry or errors contain a password, a hash, a token, or a full email (`ID-T17`) | Integration |
| The flow fails end to end through the generated REST route, step-up, and Mailpit | Cluster |

## Implementation boundaries

Ask the maintainer before these actions:

- Give password changes a password policy, a hash configuration, or a notice text that differs from signup and password reset.

## Success criteria

1. Given a password account with an open re-authentication window, When the session calls `SetPassword` with a valid new password that differs from the old one, Then the new password logs in, and the old password fails.
2. Given a provider-only account with an open re-authentication window, When the session calls `SetPassword`, Then the account can log in with the password and with its provider, and its subject, email, and links do not change.
3. Given a closed re-authentication window, When the session calls `SetPassword`, Then Identity changes nothing and returns `PermissionDenied` with the reason `REAUTHENTICATION_REQUIRED`.
4. Given a new password outside the password policy, When the session calls `SetPassword`, Then Identity returns `InvalidArgument` and changes nothing.
5. Given a subject has 3 sessions, When 1 session sets a new password, Then the other 2 sessions cannot refresh or pass `CheckSession`, and Identity deletes their device data.
6. Given a session sets a new password, When it refreshes and calls `CheckSession`, Then both succeed with its existing tokens, and its re-authentication window keeps its end time.
7. Given the account email is verified, When the session sets a new password, Then Identity queues 1 password-change notice without the password.
8. Given the account email is not verified, When the session sets a new password, Then the password changes, and Identity queues no notice.
9. Given a new password equal to the current password, When the session calls `SetPassword`, Then Identity accepts it and revokes the other sessions.
10. Given a password login for the same subject races a password change, When both finish, Then no session that the old password created stays active.
11. Given the password-hash bound is full until the deadline, When the session calls `SetPassword`, Then Identity returns `Unavailable` and changes nothing.
12. Given the password-change tests run, When a test inspects logs, traces, metrics, and errors, Then they contain no password, hash, token, or full email address.
13. Given the compromised-password check is unavailable, When the session calls `SetPassword`, Then Identity returns `Unavailable` and changes no password, session, or notice.
