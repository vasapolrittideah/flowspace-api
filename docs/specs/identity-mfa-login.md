# Spec: Identity MFA login

Module ID: `identity-mfa-login`

Status: Approved

## Objective

Make every login to an account with multi-factor authentication (MFA) prove the second factor before Identity issues tokens. A person who has the password or the provider account of a user, but not the TOTP device or a recovery code, then gets no session. The first users are API clients, and all data is disposable under [ADR-0022](../adr/0022-single-host-storage-holds-disposable-data.md).

## Scope and ADRs

The scope covers the MFA challenge that password login and provider login return for an account with MFA, the step that completes the login with a TOTP code or a recovery code, the limits of that step, and the notice when someone uses a recovery code.

This capability excludes these items:

- Turning MFA on and off, and a new set of recovery codes. The [Identity MFA enrollment](identity-mfa-enrollment.md) specification owns them.
- Re-authentication with a TOTP code or a recovery code. The MFA re-authentication module owns it.
- A remembered device that skips the second factor.
- A change to refresh, logout, or session checks.

These decisions apply:

- [ADR-0049](../adr/0049-opt-in-totp-and-recovery-codes-protect-identity-accounts.md): every password login and provider login of an account with MFA needs a second step, the challenge expires after 5 minutes, and the second step rejects `Idempotency-Key`.
- [ADR-0033](../adr/0033-identity-token-issuance-does-not-replay-responses.md) and [ADR-0034](../adr/0034-provider-login-uses-a-one-time-handoff.md): the first step keeps its existing request, limits, and retry rules.

## Contract

Use package `flowspace.identity.v1` and service `IdentityService`.

The `code` field is a string of exactly 6 ASCII digits, and leading zeroes are significant. The `recovery_code` field follows the format in [Recovery codes](identity-mfa-enrollment.md#recovery-codes) of the Identity MFA enrollment specification.

The token and session fields keep their meaning from the [Identity password login and sessions](identity-password-login-and-sessions.md) specification, and `account_deletion_due_at` keeps its meaning from the [Identity account deletion](identity-account-deletion.md) specification.

This capability adds the value `recovery-code-notice` to the `identity_email_kind` label of the `identity_email_deliveries_total` metric in the [Observability metrics and dashboards](observability-metrics-and-dashboards.md) specification.

### Methods

| RPC | Public HTTP route | Request fields | Response fields |
| --- | --- | --- | --- |
| `CompleteMfaLogin` | `POST /v1/mfa-sessions` | `mfa_challenge` (required), exactly 1 of `code` or `recovery_code` | `access_token`, `refresh_token`, `access_token_expires_at`, `refresh_token_expires_at`, `session_expires_at`, `account_deletion_due_at` |

### Method requirements

| RPC | Authentication | Retry |
| --- | --- | --- |
| `CompleteMfaLogin` | None | Rejects `Idempotency-Key` under [ADR-0049](../adr/0049-opt-in-totp-and-recovery-codes-protect-identity-accounts.md) |

### Resource fields

Login responses:

| Protobuf field | JSON field | Meaning |
| --- | --- | --- |
| `mfa_challenge` | `mfaChallenge` | A new optional field of the `CreatePasswordSession` and `CreateProviderSession` responses: the secret challenge token for an account with MFA, and unset for an account without MFA, output only |
| `mfa_challenge_expires_at` | `mfaChallengeExpiresAt` | A new optional field of the same responses: the time when the challenge expires, as a `google.protobuf.Timestamp`, and unset without a challenge, output only |

### Errors

| Condition | gRPC status | HTTP status |
| --- | --- | --- |
| Malformed request, or not exactly 1 of `code` and `recovery_code` | `InvalidArgument` | 400 |
| Unknown, expired, used, or ended challenge, or a wrong code | `Unauthenticated` | 401 |
| A limit is reached | `ResourceExhausted` | 429 |
| Identity, its database, or its limit store is unavailable | `Unavailable` | 503 |

### Notification emails

| Part | Content |
| --- | --- |
| Recipient | The verified email of the account |
| Subject | `A recovery code was used for your Flowspace account` |
| Body | The login time in UTC, such as `17 Oct 2026, 13:38 UTC`, the device label of the [Identity session management](identity-session-management.md) specification, the sign-in method, and the number of unused recovery codes. Then a sentence that tells the owner to open Flowspace, make a new set of recovery codes, and review the sessions when they do not recognize the login |

## Behavior

### First step

- When `CreatePasswordSession` or `CreateProviderSession` succeeds for an account with MFA on, Identity creates no session and issues no token. The response sets `mfa_challenge` and `mfa_challenge_expires_at`, and it leaves every other field unset.
- For an account with MFA off, both methods work as before, and the response leaves both new fields unset.
- The first step keeps its limits, its errors, and its timing. A wrong password or a failed provider login returns the same error as before, without a challenge.
- The challenge token has 256 bits from a cryptographically secure source. Identity stores only a hash of it.
- A challenge holds the subject, the sign-in method, the trusted source IP address and user agent of the first step, and its expiry. It expires 5 minutes after Identity issues it.

### Second step

- `CompleteMfaLogin` accepts the challenge with a TOTP code or a recovery code.
- A TOTP code follows the [Security and abuse](identity-mfa-enrollment.md#security-and-abuse) rules of the Identity MFA enrollment specification: the active secret, 1 step on each side, and a step later than the last accepted step.
- A recovery code matches 1 unused code of the account.
- On success, in 1 transaction, Identity uses the challenge, records the accepted step or deletes the used recovery code, and creates the session. The session uses the sign-in method and the device data of the challenge. It opens the re-authentication window, as other logins do.
- The response returns the same token and session fields as `CreatePasswordSession`. For an account in its deletion grace period, the session is a restricted session, and the response sets `account_deletion_due_at`, under the [Identity account deletion](identity-account-deletion.md) specification.
- The [Identity new device login alert](identity-new-device-login-alert.md) rules run at the second step, because the session starts there.
- A wrong code leaves the challenge in place and counts against the limits. A challenge ends after 5 wrong codes.
- A lost response leaves the session active. A retry finds the challenge used and returns `Unauthenticated`, so the client logs in again, under ADR-0033.

### Ended challenges

- These changes to the account end every open challenge of the subject in their transaction: `DisableMfa`, a password reset, a password change, a deletion request, a final deletion, and a claim of the account.
- A challenge works only when MFA is still on at the second step.

### Recovery code notice

- When a login uses a recovery code and the account email is verified, Identity queues a durable recovery-code notice in the same transaction. The notice has the content of [Notification emails](#notification-emails) and no code or link.
- If delivery fails after the commit, Identity retries the notice without a change to the session or the codes.
- When the account email is not verified, Identity queues no notice.

### Security and abuse

- Wrong codes at the second step count against the shared hourly and daily limit of each account in the [Security and abuse](identity-mfa-enrollment.md#security-and-abuse) section of the Identity MFA enrollment specification.
- Wrong codes also count against the limit of 100 wrong code guesses for each source IP address each hour, which the Identity signup and email verification specification shares across the code-submission methods under [Abuse and failure handling](identity-signup-and-email-verification.md#abuse-and-failure-handling).
- The limits use shared state across replicas, count retries, and fail closed with `Unavailable` when the limit store is unavailable.
- An unknown challenge and a wrong code return the same `Unauthenticated` error, so the error does not tell whether the challenge exists.
- A challenge token gives no access by itself. It only lets the holder try the second factor within 5 minutes.

### Data and compatibility

- A migration adds the MFA challenges and a recovery-code notice kind to the durable notice queue.
- `identity-worker` deletes each expired, used, or ended challenge within 1 hour.
- The new response fields are optional. A client that ignores them sees empty tokens only for an account with MFA on, which exists only after the enrollment module.
- A database failure, a timeout, or a cancellation before the commit leaves the challenge, the codes, the session, and the notice unchanged.
- Older API replicas issue tokens for an account with MFA without the second step. The rollout stops every older API replica before a new replica serves logins, so no login skips the second step after the rollout starts.
- Before a rollback to the version before this module, turn MFA off for every account, because older code issues tokens without the second step. Older workers ignore the recovery-code notice kind, so deliver or delete the queued recovery-code notices before the rollback of `identity-worker`.

### Diagnostics

The mail worker counts each recovery-code notice delivery in `identity_email_deliveries_total` with the kind `recovery-code-notice`. Logs, traces, metrics, errors, and security events must never contain these values:

- Challenge tokens, TOTP codes, recovery codes, their hashes, secrets, and access or refresh tokens.
- IP addresses and user agents, full email addresses, and session IDs and subject IDs in metric labels.

## Testing strategy

| Risk | Test level |
| --- | --- |
| A password or provider login of an account with MFA returns tokens, or a login without MFA returns a challenge (`ID-T09`) | Integration |
| A wrong password or a failed provider login returns a challenge, or its error or timing differs from before (`ID-T01`, `ID-T02`) | Integration |
| A challenge has less than 256 bits, or Identity stores it in plaintext (`ID-T18`) | Unit, Integration |
| A challenge works after 5 minutes, after use, after 5 wrong codes, or after an ending change, before, at, and after the boundary (`ID-T19`) | Unit, Integration |
| A TOTP code outside 1 step on each side, or at or before the last accepted step, completes a login (`ID-T03`, `ID-T04`) | Unit, Integration |
| A used recovery code completes a login, or a recovery code stays unused after a login (`ID-T04`) | Integration |
| Concurrent completions of 1 challenge create 2 sessions, or 2 logins use 1 recovery code (`ID-T12`) | Integration |
| The session has the wrong sign-in method, device data, re-authentication window, or restricted state | Integration |
| The new-device alert runs at the first step instead of the second (`ID-T21`) | Integration |
| Wrong codes bypass the account or source limits across replicas, or the limit store fails open (`ID-T03`, `ID-T16`) | Integration |
| An unknown challenge and a wrong code return different errors (`ID-T01`) | Integration |
| A request with both or neither code, or a malformed field, breaks the method (`ID-T15`) | Unit |
| The method accepts `Idempotency-Key`, or a retry after a lost response creates a second session | Integration |
| A recovery-code notice is missing for a verified email, goes to an unverified email, contains a code or a link, or has a wrong login time, device label, sign-in method, or count of unused codes (`ID-T04`, `ID-T17`) | Integration |
| A failed login transaction uses a recovery code or queues a notice, or a delivery failure loses the notice or changes the session | Integration |
| 2 challenges of 1 account complete with the same TOTP step at the same time (`ID-T04`, `ID-T12`) | Integration |
| A completion succeeds at or after the deletion due time before the final deletion runs (`ID-T19`) | Integration |
| A database failure, a timeout, or a cancellation leaves a session without a used challenge, or the opposite (`ID-T12`) | Integration |
| A challenge stays more than 1 hour after it expires or ends (`ID-T21`) | Integration |
| Telemetry or errors contain a challenge, a code, a hash, a secret, a token, an IP address, or a user agent (`ID-T17`) | Integration |
| A password login and a provider login with MFA fail end to end through the generated REST routes and a real TOTP library | Cluster |

## Implementation boundaries

Ask the maintainer before these actions:

- Change the 5-minute expiry, the 5 wrong codes for each challenge, or the limits.
- Let a login of an account with MFA issue a token without the second step.
- Return the subject, the email state, or another account field with a challenge.

## Success criteria

1. Given an account with MFA on, When the user logs in with the correct password, Then the response has a challenge and an expiry 5 minutes later, and no token.
2. Given an account with MFA on and a linked Google account, When the user completes provider login, Then the response has a challenge and no token.
3. Given an account with MFA off, When the user logs in, Then the response has tokens and no challenge.
4. Given a challenge, When the client sends a valid TOTP code, Then Identity returns tokens for a new session with the sign-in method of the first step.
5. Given a challenge, When the client sends an unused recovery code, Then Identity returns tokens, the code no longer works, and a verified email receives 1 recovery-code notice with the number of unused codes.
6. Given a challenge, When the client sends a wrong code, Then Identity returns `Unauthenticated`, and the challenge works until it expires or receives 5 wrong codes.
7. Given a challenge older than 5 minutes, When the client sends a valid code, Then Identity returns `Unauthenticated`.
8. Given a used challenge after a lost response, When the client sends it again, Then Identity returns `Unauthenticated`, and the first session stays active.
9. Given an open challenge, When the user turns MFA off or resets the password, Then the challenge returns `Unauthenticated`.
10. Given an account in its deletion grace period with MFA on, When the user completes both steps, Then the session is restricted, and the response sets `account_deletion_due_at`.
11. Given an account with a verified email and an unexpired known device, When the user completes both steps from a new device, Then Identity queues 1 new-device notice.
12. Given 10 wrong codes for an account within 1 hour across enrollment and login, When the client sends another code, Then Identity returns `ResourceExhausted`.
13. Given 2 concurrent completions of 1 challenge, When both commit, Then Identity creates 1 session.
14. Given the MFA login tests run, When a test inspects logs, traces, metrics, and errors, Then they contain no challenge, code, hash, secret, token, IP address, or user agent.
