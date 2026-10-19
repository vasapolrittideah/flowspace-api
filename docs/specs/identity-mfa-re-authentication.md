# Spec: Identity MFA re-authentication

Module ID: `identity-mfa-re-authentication`

Status: Draft

## Objective

Let a signed-in user with multi-factor authentication (MFA) open the re-authentication window with a TOTP code or a recovery code, and make these the only re-authentication methods for that account. A person who has a session and the password, the provider account, or the mailbox of the user, but not the second factor, then cannot turn MFA off or make another sensitive change. The first users are API clients, and all data is disposable under [ADR-0022](../adr/0022-single-host-storage-holds-disposable-data.md).

## Scope and ADRs

The scope covers 1 new re-authentication method, the restriction of the existing methods for an account with MFA, and the notice when someone uses a recovery code for re-authentication.

This capability excludes these items:

- The re-authentication window, its 10-minute length, and the list of sensitive methods. The [Identity re-authentication](identity-re-authentication.md) specification owns them.
- Turning MFA on and off, and the login step. The [Identity MFA enrollment](identity-mfa-enrollment.md) and [Identity MFA login](identity-mfa-login.md) specifications own them.

These decisions apply:

- [ADR-0049](../adr/0049-opt-in-totp-and-recovery-codes-protect-identity-accounts.md): an account with MFA re-authenticates only with a TOTP code or a recovery code, and a notice goes to a verified email each time someone uses a recovery code.
- [ADR-0045](../adr/0045-provider-re-authentication-reuses-the-one-time-handoff.md): provider re-authentication stays for accounts without MFA.

## Contract

Use package `flowspace.identity.v1` and service `IdentityService`.

The `code` field is a string of exactly 6 ASCII digits, and leading zeroes are significant. The `recovery_code` field follows the format in [Recovery codes](identity-mfa-enrollment.md#recovery-codes) of the Identity MFA enrollment specification. The response field `reauthentication_expires_at` keeps its meaning from the Identity re-authentication specification.

The existing re-authentication methods return a new `FailedPrecondition` error for an account with MFA. It carries an `ErrorInfo` detail whose reason is `MFA_REAUTHENTICATION_REQUIRED` and whose domain is `identity.flowspace`.

### Methods

| RPC | Public HTTP route | Request fields | Response fields |
| --- | --- | --- | --- |
| `ReauthenticateWithMfa` | `POST /v1/mfa-reauthentications` | Exactly 1 of `code` or `recovery_code` | `reauthentication_expires_at` |

### Method requirements

| RPC | Authentication | Retry |
| --- | --- | --- |
| `ReauthenticateWithMfa` | Bearer access token | Safe to retry |

### Errors

| Condition | gRPC status | HTTP status |
| --- | --- | --- |
| Malformed request, or not exactly 1 of `code` and `recovery_code` | `InvalidArgument` | 400 |
| Wrong code, a TOTP step that Identity already accepted, or a used recovery code | `InvalidArgument` | 400 |
| Missing, invalid, or inactive session | `Unauthenticated` | 401 |
| `ReauthenticateWithMfa` for an account without MFA | `FailedPrecondition` | 400 |
| `ReauthenticateWithPassword`, `RequestReauthenticationCode`, `ReauthenticateWithCode`, `StartProviderReauthentication`, or `CompleteProviderReauthentication` for an account with MFA | `FailedPrecondition` | 400 |
| A limit is reached | `ResourceExhausted` | 429 |
| Identity, its database, or its limit store is unavailable | `Unavailable` | 503 |

## Behavior

### MFA re-authentication

- `ReauthenticateWithMfa` accepts a TOTP code under the [Security and abuse](identity-mfa-enrollment.md#security-and-abuse) rules of the Identity MFA enrollment specification, or 1 unused recovery code of the account.
- When the window of the current session is closed, a success records the accepted step or deletes the used recovery code, sets the re-authentication time of the session to the commit time, and returns the new end time, in 1 transaction.
- When the window is open, Identity returns the current end time without a check of the code, without a used step or code, and without a notice. A retry after a lost response then has no additional effect.
- A wrong code changes nothing and counts against the limits.
- The method changes no other session, no token, and no account data.

### Restricted methods

- For an account with MFA on, `ReauthenticateWithPassword`, `RequestReauthenticationCode`, `ReauthenticateWithCode`, `StartProviderReauthentication`, and `CompleteProviderReauthentication` return `FailedPrecondition` with the reason `MFA_REAUTHENTICATION_REQUIRED`. They change nothing, send no code, and count against no limit.
- Each method makes sure that MFA is off in its own transaction. A provider attempt or an email code that started before MFA turned on cannot open the window after it.
- For an account with MFA off, these methods work as the Identity re-authentication specification states, and `ReauthenticateWithMfa` returns `FailedPrecondition`.
- A window that was open when MFA turned on stays open until its end, because the session proved the user within the last 10 minutes.
- A login that passes the second factor still opens the window, under the Identity MFA login specification.

### Recovery code notice

- When a re-authentication uses a recovery code and the account email is verified, Identity queues a durable recovery-code notice in the same transaction.
- The notice uses the [Notification emails](identity-mfa-login.md#notification-emails) of the Identity MFA login specification, with the time of the re-authentication, the device label and the sign-in method of the current session, and the number of unused codes.
- When the account email is not verified, Identity queues no notice. If delivery fails after the commit, Identity retries the notice without a change to the window or the codes.

### Security and abuse

- Wrong codes count against the shared hourly and daily limit of each account in the Identity MFA enrollment specification, and against the limit of 100 wrong code guesses for each source IP address each hour under [Abuse and failure handling](identity-signup-and-email-verification.md#abuse-and-failure-handling).
- The limits use shared state across replicas, count retries, and fail closed with `Unavailable` when the limit store is unavailable.
- A failed re-authentication does not revoke the session or lock the account.

### Data and compatibility

- This module adds no table and no notice kind. It uses the secret, the last accepted step, the recovery codes, and the recovery-code notice of the MFA enrollment and MFA login modules.
- A database failure, a timeout, or a cancellation before the commit leaves the window, the step, the codes, and the notice unchanged.
- Older API replicas accept the existing methods for an account with MFA. The rollout stops every older API replica before a new replica serves re-authentication requests.
- Before a rollback to the version before this module, turn MFA off for every account, because older code lets the existing methods open the window.

### Diagnostics

Logs, traces, metrics, errors, and security events record the outcome of each re-authentication and each refused method. They must never contain these values:

- TOTP codes, recovery codes, their hashes, secrets, and tokens.
- IP addresses and user agents, full email addresses, and session IDs and subject IDs in metric labels.

## Testing strategy

| Risk | Test level |
| --- | --- |
| A TOTP code outside 1 step on each side, or at or before the last accepted step, opens the window (`ID-T03`, `ID-T04`) | Unit, Integration |
| A used recovery code opens the window, or a recovery code stays unused after a success (`ID-T04`) | Integration |
| A closed-window success returns an end time other than the earlier of 10 minutes after the commit and the idle or absolute expiry of the session (`ID-T19`) | Unit, Integration |
| A call with an open window tries to make sure that the code is valid, uses a code, moves the end time, or queues a notice (`ID-T19`) | Integration |
| An existing method opens the window, sends a code, or counts against a limit for an account with MFA (`ID-T09`) | Integration |
| A provider attempt or an email code that started before MFA turned on opens the window after it (`ID-T12`) | Integration |
| `ReauthenticateWithMfa` succeeds for an account without MFA, or an existing method fails for it | Integration |
| A window that was open when MFA turned on closes early, or stays open after its end (`ID-T19`) | Integration |
| Concurrent calls use 1 TOTP step or 1 recovery code twice (`ID-T04`, `ID-T12`) | Integration |
| Wrong codes bypass the account or source limits across replicas, or the limit store fails open (`ID-T03`, `ID-T16`) | Integration |
| A request with both or neither code, or a malformed field, breaks the method (`ID-T15`) | Unit |
| A recovery-code notice is missing for a verified email, goes to an unverified email, has wrong content, or contains a code or a link (`ID-T04`, `ID-T17`) | Integration |
| A database failure, a timeout, or a cancellation leaves a used code without an open window, or the opposite (`ID-T12`) | Integration |
| Telemetry or errors contain a code, a hash, a secret, a token, an IP address, or a user agent (`ID-T17`) | Integration |
| `DisableMfa` with a window from `ReauthenticateWithMfa` fails end to end through the generated REST routes and a real TOTP library | Cluster |

## Implementation boundaries

Ask the maintainer before these actions:

- Let a method other than a TOTP code, a recovery code, or a login with the second factor open the window for an account with MFA.
- Change the limits or the error reason.

## Success criteria

1. Given an account with MFA, a closed window, and a session at least 10 minutes before its idle and absolute expiry, When the session calls `ReauthenticateWithMfa` with a valid TOTP code, Then Identity returns an end time 10 minutes after the commit.
2. Given an account with MFA and a closed window, When the session uses an unused recovery code, Then the window opens, the code no longer works, and a verified email receives 1 recovery-code notice.
3. Given an open window, When the session calls `ReauthenticateWithMfa` again, Then Identity returns the same end time and uses no code.
4. Given an account with MFA, When the session calls `ReauthenticateWithPassword`, `ReauthenticateWithCode`, or `CompleteProviderReauthentication`, Then Identity returns `FailedPrecondition` with the reason `MFA_REAUTHENTICATION_REQUIRED`, and the window stays closed.
5. Given an account with MFA, When the session calls `RequestReauthenticationCode`, Then Identity sends no code.
6. Given an email code that Identity issued before MFA turned on, When the session submits it after, Then Identity returns `FailedPrecondition`.
7. Given an account without MFA, When the session calls `ReauthenticateWithMfa`, Then Identity returns `FailedPrecondition`.
8. Given a closed window and a wrong code, When the session calls `ReauthenticateWithMfa`, Then Identity returns `InvalidArgument`, and the window stays closed.
9. Given a window from `ReauthenticateWithMfa`, When the session calls `DisableMfa`, Then MFA turns off.
10. Given 10 wrong codes for an account within 1 hour across enrollment, login, and re-authentication, When the session sends another code, Then Identity returns `ResourceExhausted`.
11. Given the MFA re-authentication tests run, When a test inspects logs, traces, metrics, and errors, Then they contain no code, hash, secret, token, IP address, or user agent.
