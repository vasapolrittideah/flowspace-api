# Spec: Identity MFA remembered device

Module ID: `identity-mfa-remembered-device`

Status: Draft

## Objective

Let a user with multi-factor authentication (MFA) skip the TOTP step on a device that passed MFA within the last 30 days. The user asks Identity to remember the device at the end of an MFA login and sends the remembered-device token with later logins from that device. The first users are API clients, and all data is disposable under [ADR-0022](../adr/0022-single-host-storage-holds-disposable-data.md).

## Scope and ADRs

The scope covers the creation of a remembered-device token, its use at password login and provider login, its expiry and revocation, and the count of remembered devices in the MFA status.

This capability excludes these items:

- A list of single devices and the revocation of 1 device.
- An extension of the token on use.
- Skipping the second factor at re-authentication.
- The storage of the token in the web frontend.

These decisions apply:

- [ADR-0050](../adr/0050-remembered-devices-skip-the-mfa-login-step-for-30-days.md): the token, its 30-day fixed life, the fallback to the MFA challenge, the closed re-authentication window, and the revoking changes.
- [ADR-0049](../adr/0049-opt-in-totp-and-recovery-codes-protect-identity-accounts.md): the MFA login step that the token skips.

## Contract

Use package `flowspace.identity.v1` and service `IdentityService`.

The `remembered_device_token` request field is optional, and the existing methods ignore it for an account without MFA.

### Methods

| RPC | Public HTTP route | Request fields | Response fields |
| --- | --- | --- | --- |
| `RevokeRememberedDevices` | `POST /v1/remembered-device-revocations` | None | None |

### Method requirements

| RPC | Authentication | Retry |
| --- | --- | --- |
| `RevokeRememberedDevices` | Bearer access token | Safe to retry |

### Resource fields

Changed requests and responses:

| Protobuf field | JSON field | Meaning |
| --- | --- | --- |
| `remember_device` | `rememberDevice` | A new optional field of the `CompleteMfaLogin` request: `true` to ask for a remembered-device token, input only |
| `remembered_device_token` | `rememberedDeviceToken` | A new optional field of the `CompleteMfaLogin` response, set only when the request asked for it: the secret token, output only. The same name is a new optional field of the `CreatePasswordSession` and `CreateProviderSession` requests, input only |
| `remembered_device_expires_at` | `rememberedDeviceExpiresAt` | A new optional field of the `CompleteMfaLogin` response: the time when the token expires, as a `google.protobuf.Timestamp`, and unset without a token, output only |
| `remembered_devices` | `rememberedDevices` | A new field of the `GetMfaStatus` response: the number of unexpired, unrevoked remembered-device tokens of the account, and 0 when MFA is off, output only |

### Errors

| Condition | gRPC status | HTTP status |
| --- | --- | --- |
| Malformed request, or a `remembered_device_token` longer than 128 characters | `InvalidArgument` | 400 |
| Missing, invalid, or inactive session | `Unauthenticated` | 401 |
| A client calls `RevokeRememberedDevices` outside the re-authentication window | `PermissionDenied` | 403 |
| Identity or its database is unavailable | `Unavailable` | 503 |

## Behavior

### Creating a token

- When `CompleteMfaLogin` succeeds and the request sets `remember_device`, Identity creates a token in the same transaction as the session. The response sets `remembered_device_token` and `remembered_device_expires_at`.
- The token has 256 bits from a cryptographically secure source. Identity stores only a hash of it with the subject, the creation time, and the expiry. It stores no IP address, user agent, or session ID.
- The token expires 30 days after the commit of the login that created it. A use never moves the expiry.
- An account keeps at most 20 unexpired, unrevoked tokens. A new token beyond 20 revokes the oldest unexpired, unrevoked token of the account in the same transaction.
- A lost response loses the token. The next login from the device asks for the second factor again.

### Using a token

- When `CreatePasswordSession` or `CreateProviderSession` succeeds for an account with MFA and the request has a token that matches an unexpired, unrevoked token of the same subject, Identity creates the session and returns tokens without an MFA challenge.
- When the token is missing, unknown, expired, revoked, or belongs to another account, the method returns the MFA challenge, as the [Identity MFA login](identity-mfa-login.md) specification states. The response is the same for each of these cases.
- The first step keeps its limits, its errors, and its timing. A wrong password or a failed provider login returns the same error as before, and Identity does not look up the token.
- A session from a token does not open the re-authentication window. Before a sensitive method, the user calls `ReauthenticateWithMfa`, as the [Identity MFA re-authentication](identity-mfa-re-authentication.md) specification states.
- The [Identity new device login alert](identity-new-device-login-alert.md) rules run for the session as for any login.
- A use of a token does not change the token.

### Revocation

- In their own transactions, these changes revoke every token of the account: a password change, a password reset, `DisableMfa`, `RegenerateRecoveryCodes`, a deletion request, and a final deletion.
- A logout, a session revocation, a provider unlink, and an email change revoke no token.
- `RevokeRememberedDevices` is a sensitive method under the [Identity re-authentication](identity-re-authentication.md) specification. It revokes every token of the account and keeps every session. A retry finds no token and returns success without a change.
- A revoked token fails from the commit on.

### Security and abuse

- The token is a secret like a refresh token. It appears only in request and response bodies, never in a URL, a header that a proxy logs, or a cookie that Identity sets.
- A wrong token adds no limit bucket, because the password or the provider login before it already counts against the login limits.
- A person who has the password or the provider account and an unexpired token signs in without the second factor. The maintainer accepted this risk in ADR-0050.

### Data and compatibility

- A migration adds a remembered-device table with a unique token hash and an index by subject.
- `identity-worker` deletes each expired or revoked token within 24 hours.
- The new fields are optional. An older client that never sends a token always gets the MFA challenge.
- A database failure, a timeout, or a cancellation before the commit leaves the session, the tokens, and the revocations unchanged.
- Older API replicas ignore the token and return the MFA challenge, but they do not revoke tokens at a revoking change. The rollout stops every older API replica before a new replica serves logins or revoking changes.
- Before a rollback to the version before this module, delete every remembered-device token, so that no token survives a revoking change that older code serves.

### Diagnostics

Logs, traces, metrics, errors, and security events must never contain these values:

- Remembered-device tokens, their hashes, and access or refresh tokens.
- Full email addresses, and session IDs and subject IDs in metric labels.

## Testing strategy

| Risk | Test level |
| --- | --- |
| A token has less than 256 bits, or Identity stores it in plaintext, with an IP address, or with a user agent (`ID-T18`, `ID-T21`) | Unit, Integration |
| A token works after 30 days, or a use moves its expiry, before, at, and after the boundary (`ID-T19`) | Unit, Integration |
| A token of another account, or a revoked token, skips the challenge (`ID-T09`) | Integration |
| A missing, unknown, expired, revoked, or foreign token gives a response other than the normal challenge (`ID-T01`) | Integration |
| A wrong password or a failed provider login with a valid token returns a session, or its error or timing differs (`ID-T02`) | Integration |
| A session from a token opens the re-authentication window (`ID-T09`) | Integration |
| A revoking change leaves a token valid, or a logout, a session revocation, an unlink, or an email change revokes one | Integration |
| `RevokeRememberedDevices` succeeds outside the window, ends a session, or gives a second effect on retry | Integration |
| A 21st token keeps more than 20 unexpired tokens, or revokes a token other than the oldest | Integration |
| Concurrent logins with 1 token, or a login racing a revocation, give a session from a revoked token (`ID-T12`) | Integration |
| `GetMfaStatus` returns a wrong count, also right after a revocation or an eviction | Integration |
| A long or malformed token breaks a login (`ID-T15`) | Unit |
| A login with a valid token bypasses the password or provider login limits across replicas, or succeeds when the limit store is unavailable (`ID-T02`, `ID-T16`) | Integration |
| A rollback that does not delete the tokens, or a re-upgrade, lets a token survive a revoking change | Integration |
| A token stays stored more than 24 hours after it expires or is revoked (`ID-T21`) | Integration |
| A database failure, a timeout, or a cancellation leaves a session without its token, or the opposite (`ID-T12`) | Integration |
| Telemetry or errors contain a remembered-device token, a hash, or a token (`ID-T17`) | Integration |
| A login that remembers a device and a later login that skips MFA fail end to end through the generated REST routes | Cluster |

## Implementation boundaries

Ask the maintainer before these actions:

- Change the 30-day life, the limit of 20 tokens, or the revoking changes.
- Store an IP address, a user agent, or a device name with a token.
- Let a session from a token open the re-authentication window.

## Success criteria

1. Given a challenge, When the client completes it with a valid code and `remember_device=true`, Then Identity returns session tokens, a remembered-device token, and an expiry 30 days later.
2. Given an unexpired token, When the user logs in with the correct password and the token, Then Identity returns session tokens without a challenge.
3. Given an unexpired token, When the user completes a Google login with the token, Then Identity returns session tokens without a challenge.
4. Given an expired, revoked, unknown, or foreign token, When the user logs in with it, Then Identity returns the MFA challenge.
5. Given a session from a token, When the session calls `DisableMfa`, Then Identity returns `PermissionDenied` with the reason `REAUTHENTICATION_REQUIRED`.
6. Given an unexpired token, When the user changes the password, resets it, or makes a new set of recovery codes, Then the token returns the challenge.
7. Given an unexpired token, When the user turns MFA off and then on again, Then `remembered_devices` is 0 after the first change, and the old token returns the challenge after the second.
8. Given an unexpired token, When the session logs out, Then the token still skips the challenge.
9. Given an open re-authentication window, When the session calls `RevokeRememberedDevices`, Then every token of the account returns the challenge, and every session stays active.
10. Given 2 unexpired tokens, When the session calls `GetMfaStatus`, Then `remembered_devices` is 2.
11. Given 20 unexpired tokens, When a login creates another token, Then the oldest token returns the challenge, and `remembered_devices` is 20.
12. Given the remembered-device tests run, When a test inspects logs, traces, metrics, and errors, Then they contain no remembered-device token, hash, or session token.
