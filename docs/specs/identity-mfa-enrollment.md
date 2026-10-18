# Spec: Identity MFA enrollment

Module ID: `identity-mfa-enrollment`

Status: Draft

## Objective

Let a signed-in user turn multi-factor authentication (MFA) on and off for their account, and keep a set of recovery codes. After this module, an account can hold a TOTP secret and 10 recovery codes, and the user can read whether MFA is on. The MFA login and MFA re-authentication modules later ask for the second factor. The first users are API clients, and all data is disposable under [ADR-0022](../adr/0022-single-host-storage-holds-disposable-data.md).

## Scope and ADRs

The scope covers the start and the confirmation of MFA, the status of MFA, turning MFA off, a new set of recovery codes, the storage of the secret and the codes, the shared limit for TOTP and recovery codes, and the notices.

This capability excludes these items:

- The second step of login and the re-authentication with a TOTP code or a recovery code. The MFA login and MFA re-authentication modules specify them.
- A remembered device, SMS, WebAuthn, and passkeys.
- A recovery for a user who loses the TOTP device and every recovery code.
- A rotation of the encryption key.

These decisions apply:

- [ADR-0049](../adr/0049-opt-in-totp-and-recovery-codes-protect-identity-accounts.md): opt-in TOTP and recovery codes, their values, and the scoped exception to ADR-0011.
- [ADR-0035](../adr/0035-identity-signup-security-and-mail-delivery.md): Identity queues each notice durably and sends it only to the verified email of the account.

## Contract

Use package `flowspace.identity.v1` and service `IdentityService`.

The `code` field is a string of exactly 6 ASCII digits, and leading zeroes are significant.

The `ConfirmMfaEnrollment` and `RegenerateRecoveryCodes` responses have 1 field. `recovery_codes` (JSON `recoveryCodes`) holds 10 new recovery codes, each in the form `xxxxx-xxxxx`, output only.

This capability adds the values `mfa-on-notice` and `mfa-off-notice` to the `identity_email_kind` label of the `identity_email_deliveries_total` metric in the [Observability metrics and dashboards](observability-metrics-and-dashboards.md) specification.

### Methods

| RPC | Public HTTP route | Request fields | Response fields |
| --- | --- | --- | --- |
| `GetMfaStatus` | `GET /v1/mfa-status` | None | `enabled`, `remaining_recovery_codes` |
| `StartMfaEnrollment` | `POST /v1/mfa-enrollments` | None | `secret`, `otpauth_uri`, `expires_at` |
| `ConfirmMfaEnrollment` | `POST /v1/mfa-activations` | `code` (required) | `recovery_codes` |
| `DisableMfa` | `POST /v1/mfa-deactivations` | None | None |
| `RegenerateRecoveryCodes` | `POST /v1/recovery-code-sets` | None | `recovery_codes` |

### Method requirements

| RPC | Authentication | Retry |
| --- | --- | --- |
| `GetMfaStatus` | Bearer access token | Safe to retry |
| `StartMfaEnrollment` | Bearer access token | Safe to retry |
| `ConfirmMfaEnrollment` | Bearer access token | Rejects `Idempotency-Key` under [ADR-0049](../adr/0049-opt-in-totp-and-recovery-codes-protect-identity-accounts.md) |
| `DisableMfa` | Bearer access token | Safe to retry |
| `RegenerateRecoveryCodes` | Bearer access token | Rejects `Idempotency-Key` under [ADR-0049](../adr/0049-opt-in-totp-and-recovery-codes-protect-identity-accounts.md) |

### Resource fields

MFA status:

| Protobuf field | JSON field | Meaning |
| --- | --- | --- |
| `enabled` | `enabled` | `true` when MFA is on for the account, output only |
| `remaining_recovery_codes` | `remainingRecoveryCodes` | The number of unused recovery codes, from 0 to 10, and 0 when MFA is off, output only |

MFA enrollment:

| Protobuf field | JSON field | Meaning |
| --- | --- | --- |
| `secret` | `secret` | The pending TOTP secret as 32 unpadded base32 characters, output only |
| `otpauth_uri` | `otpauthUri` | An `otpauth://totp/` URI with the issuer `Flowspace`, the account email as the label, and the secret, for a QR code, output only |
| `expires_at` | `expiresAt` | The time when the pending secret expires, as a `google.protobuf.Timestamp`, output only |

### Errors

| Condition | gRPC status | HTTP status |
| --- | --- | --- |
| Malformed request or malformed code | `InvalidArgument` | 400 |
| Wrong code, or no unexpired pending secret | `InvalidArgument` | 400 |
| Missing, invalid, or inactive session | `Unauthenticated` | 401 |
| A client calls `StartMfaEnrollment`, `ConfirmMfaEnrollment`, `DisableMfa`, or `RegenerateRecoveryCodes` outside the re-authentication window | `PermissionDenied` | 403 |
| `StartMfaEnrollment` or `ConfirmMfaEnrollment` when MFA is on, or `RegenerateRecoveryCodes` when MFA is off | `FailedPrecondition` | 400 |
| A limit is reached | `ResourceExhausted` | 429 |
| Identity, its database, or its limit store is unavailable | `Unavailable` | 503 |

## Behavior

### Turning MFA on

- `StartMfaEnrollment`, `ConfirmMfaEnrollment`, `DisableMfa`, and `RegenerateRecoveryCodes` are sensitive methods under the [Identity re-authentication](identity-re-authentication.md) specification. Each makes sure that the session of the access token has an open re-authentication window in the transaction of its change.
- `StartMfaEnrollment` generates a 160-bit secret from a cryptographically secure source and stores it encrypted as the pending secret of the account. The pending secret expires 10 minutes after Identity issues it. While an unexpired pending secret exists, a new start returns the same secret and expiry without a change. After it expires or ends, a new start issues a new secret.
- `ConfirmMfaEnrollment` accepts a code that the pending secret gives for the current 30-second step or 1 step on each side, under ADR-0049. In 1 transaction, Identity makes the pending secret the active secret, records the step of the code as the last accepted step, creates 10 recovery codes, and queues an MFA-on notice.
- A wrong code leaves the pending secret in place and counts against the limits.
- Any account can turn MFA on, including a provider-only account and an account with an unverified email.

### Status

`GetMfaStatus` returns whether MFA is on and how many recovery codes are unused. It never returns the secret or a recovery code.

### Recovery codes

- Each recovery code has 10 characters from the 31 lowercase letters and digits that exclude `0`, `o`, `1`, `i`, and `l`. Identity shows it as 2 groups of 5 characters with a hyphen.
- Identity accepts a recovery code without regard to case, spaces, or hyphens.
- Identity stores only a keyed hash of each code. The hash key is an Identity secret. Identity never logs, stores, or returns a plaintext code after the response that creates it.
- `RegenerateRecoveryCodes` deletes every old code and creates 10 new codes in 1 transaction. An old code fails from the commit on.
- A lost `ConfirmMfaEnrollment` or `RegenerateRecoveryCodes` response loses the codes. The client calls `RegenerateRecoveryCodes` to get a new set. A retry of `ConfirmMfaEnrollment` returns `FailedPrecondition`, because MFA is already on.

### Turning MFA off

- When MFA is on, `DisableMfa` deletes the active secret and every recovery code in 1 transaction. It queues an MFA-off notice and revokes no session.
- When MFA is off, `DisableMfa` deletes any pending secret and returns success without a notice. A retry returns success without a change.
- `DisableMfa` keeps the last accepted step of the account.
- To move to a new authenticator app, the user turns MFA off and then on again.

### Re-authentication methods

The methods of the [Identity re-authentication](identity-re-authentication.md) specification open the window for these sensitive methods. The MFA re-authentication module later limits the methods for an account with MFA.

### Notices

- When the account email is verified, the confirmation queues an MFA-on notice, and `DisableMfa` queues an MFA-off notice, in the same transaction as the change. A notice contains the time of the change and no secret, code, or link.
- When the account email is not verified, Identity changes MFA and queues no notice.
- If delivery fails after the commit, Identity retries the notice without a change to MFA.

### Security and abuse

- Identity encrypts each active and pending secret with AES-256-GCM and an Identity key, and it records the key ID with the ciphertext. The key never leaves Identity.
- Wrong TOTP codes and wrong recovery codes share 1 limit for each account: 5 wrong codes for 1 pending secret, 10 for each account each hour, and 20 for each account each day. The MFA login and MFA re-authentication modules count against the same hourly and daily limit.
- The limits use shared state across replicas, count retries, and fail closed with `Unavailable` when the limit store is unavailable.
- After 5 wrong codes, the pending secret ends, and the user starts again.
- Identity keeps the last accepted step for each account and accepts a code only for a later step, so a code that a person saw cannot work a second time. Turning MFA off and on again keeps this step, so the rule holds across secrets.

### Data and compatibility

- A migration adds the active secret, the pending secret with its expiry, the last accepted step, and the recovery code hashes for each account. It adds 2 notice kinds to the durable notice queue.
- Each change and its notice commit in 1 transaction. A database failure, a timeout, or a cancellation before the commit leaves MFA, the codes, and the notices unchanged.
- The final deletion and the account claim under the [Identity account deletion](identity-account-deletion.md) specification delete the secret and the codes of the old subject.
- Before a rollback to the version before this module, turn MFA off for every account, because older code does not ask for the second factor.

### Diagnostics

The mail worker counts each notice delivery in `identity_email_deliveries_total` of the [Observability metrics and dashboards](observability-metrics-and-dashboards.md) specification with the kind `mfa-on-notice` or `mfa-off-notice`. Logs, traces, metrics, errors, and security events must never contain these values:

- Secrets, `otpauth://` URIs, TOTP codes, recovery codes, their hashes, encryption keys, and tokens.
- Full email addresses, and session IDs and subject IDs in metric labels.

## Testing strategy

| Risk | Test level |
| --- | --- |
| A sensitive method succeeds outside the re-authentication window (`ID-T09`) | Integration |
| A secret has less than 160 bits, does not match RFC 6238 test vectors, or accepts a step outside 1 step on each side, before, at, and after the boundary (`ID-T19`) | Unit |
| A pending secret works after 10 minutes, after `DisableMfa`, or after 5 wrong codes (`ID-T03`, `ID-T19`) | Unit, Integration |
| A used step works a second time, also after `DisableMfa` and a new enrollment within the same step (`ID-T04`) | Unit, Integration |
| A start with an unexpired pending secret returns another secret or another expiry | Integration |
| A start or a confirmation succeeds when MFA is on, or a new set succeeds when MFA is off | Integration |
| A confirmation retry after a lost response returns codes instead of `FailedPrecondition`, a new set after a lost response leaves the lost set valid, or either method accepts `Idempotency-Key` | Integration |
| `DisableMfa` with only a pending secret leaves the secret or queues a notice | Integration |
| A malformed code, a long code, or a request with unknown fields breaks a method or reaches the database (`ID-T15`) | Unit |
| Identity stores a secret in plaintext or without a key ID, or the secret is readable without the key (`ID-T18`) | Integration |
| A recovery code has the wrong alphabet, length, or entropy, or Identity stores it in plaintext (`ID-T18`) | Unit, Integration |
| Recovery-code input fails for another case, spaces, or hyphens | Unit |
| A new set leaves an old code valid, or a status call returns a secret or a code | Integration |
| `DisableMfa` leaves a secret or a code, removes the last accepted step, or revokes a session | Integration |
| Wrong codes bypass the per-secret, hourly, or daily limit across replicas, or the limit store fails open (`ID-T03`, `ID-T16`) | Integration |
| Concurrent confirmations create 2 sets of codes, or a confirmation races `DisableMfa` (`ID-T12`) | Integration |
| A notice is missing for a verified email, goes to an unverified email, or contains a secret, a code, or a link (`ID-T04`, `ID-T17`) | Integration |
| A database failure, a timeout, or a cancellation leaves a change without its notice, or the opposite (`ID-T12`) | Integration |
| The final deletion or a claim leaves the secret or the codes of the old subject (`ID-T21`) | Integration |
| Telemetry or errors contain a secret, a URI, a code, a hash, a key, or a token (`ID-T17`) | Integration |
| The enrollment fails end to end through the generated REST routes and a real TOTP library | Cluster |

## Implementation boundaries

Ask the maintainer before these actions:

- Change a TOTP value, the number or the format of the recovery codes, the 10-minute expiry, or the limits.
- Store a secret or a recovery code in another form, or return it after its first response. This rule excludes the same unexpired pending secret that `StartMfaEnrollment` returns again, as Turning MFA on states. It never excludes an active secret or a recovery code.
- Let a status method return the secret or a code.

## Success criteria

1. Given an open re-authentication window, MFA off, and no unexpired pending secret, When the session calls `StartMfaEnrollment`, Then Identity returns a 32-character secret, an `otpauth://` URI, and an expiry 10 minutes later.
2. Given a pending secret, When the session confirms a valid code, Then MFA is on, Identity returns 10 recovery codes, and `GetMfaStatus` returns `enabled=true` and `remaining_recovery_codes=10`.
3. Given an unexpired pending secret, When the session calls `StartMfaEnrollment` again, Then Identity returns the same secret and expiry.
4. Given a pending secret, When the session confirms a wrong code, Then MFA stays off, and the pending secret stays until it expires or receives 5 wrong codes.
5. Given a closed re-authentication window, When the session calls any of the 4 sensitive methods, Then Identity changes nothing and returns `PermissionDenied` with the reason `REAUTHENTICATION_REQUIRED`.
6. Given MFA on, When the session calls `StartMfaEnrollment`, Then Identity returns `FailedPrecondition`.
7. Given a lost confirmation response, When the client retries `ConfirmMfaEnrollment`, Then Identity returns `FailedPrecondition`, and `RegenerateRecoveryCodes` returns a new set.
8. Given MFA on, When the session calls `RegenerateRecoveryCodes`, Then Identity returns 10 new codes, and every old code fails.
9. Given MFA on, When the session calls `DisableMfa`, Then MFA is off, Identity deletes the secret and the codes, and every session stays active.
10. Given MFA off and a pending secret, When the session calls `DisableMfa`, Then Identity deletes the pending secret and queues no notice.
11. Given a verified email, When MFA turns on and then off, Then Identity queues 1 MFA-on notice and 1 MFA-off notice without a secret, a code, or a link.
12. Given an unverified email, When MFA turns on, Then Identity queues no notice.
13. Given 10 wrong codes for an account within 1 hour, When the session submits another code, Then Identity returns `ResourceExhausted`.
14. Given a code that Identity accepted, When the same step arrives again, also after MFA turns off and on, Then Identity rejects it.
15. Given a deleted or claimed account, When the transaction commits, Then the old subject has no secret and no recovery code.
16. Given the enrollment tests run, When a test inspects logs, traces, metrics, and errors, Then they contain no secret, URI, code, hash, key, or token.
