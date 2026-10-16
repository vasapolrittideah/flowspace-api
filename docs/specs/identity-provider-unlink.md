# Spec: Identity provider unlink

Module ID: `identity-provider-unlink`

Status: Draft

## Objective

Let a signed-in user remove the Google or GitHub account that is linked to their Flowspace account. After the unlink, that provider account no longer signs in to the Flowspace account, and the other sessions that signed in through it end. The first users are API clients, and all data is disposable under [ADR-0022](../adr/0022-single-host-storage-holds-disposable-data.md).

An account keeps at least 1 way to sign in, so a user cannot remove the last one.

## Scope and ADRs

The scope covers 1 method that removes the link of 1 provider from the account of the caller, the rule that keeps 1 sign-in method, and the revocation of the sessions of that provider. It also covers a database rule that allows 1 link for each provider on each account.

This capability excludes these items:

- A method that links a provider. The `identity-provider-link` module adds it later.
- A method that lists the links of an account.
- An email notice after an unlink.
- The removal of a password from an account.

These decisions apply:

- [ADR-0013](../adr/0013-acting-identity-comes-from-the-token.md): the account comes from the access token, never from the request.
- [ADR-0031](../adr/0031-flowspace-owns-authentication-and-revocable-sessions.md): Identity owns provider links and sessions, and a revocation is durable before success.

## Contract

Use package `flowspace.identity.v1` and service `IdentityService`.

The `provider` field uses the existing `Provider` enum of provider login and accepts only `PROVIDER_GOOGLE` and `PROVIDER_GITHUB`. `PROVIDER_UNSPECIFIED` and unknown values are malformed.

### Methods

| RPC | Public HTTP route | Request fields | Response fields |
| --- | --- | --- | --- |
| `UnlinkProvider` | `POST /v1/provider-unlinks` | `provider` (required) | None |

### Method requirements

| RPC | Authentication | Retry |
| --- | --- | --- |
| `UnlinkProvider` | Bearer access token | Safe to retry |

### Errors

| Condition | gRPC status | HTTP status |
| --- | --- | --- |
| Malformed request or provider value | `InvalidArgument` | 400 |
| Missing, invalid, or inactive session | `Unauthenticated` | 401 |
| The provider is the last sign-in method of the account | `FailedPrecondition` | 400 |
| A client calls `UnlinkProvider` outside the re-authentication window | `PermissionDenied` | 403 |
| The account has no link for the provider | `NotFound` | 404 |
| Identity or its database is unavailable | `Unavailable` | 503 |

## Behavior

### Unlinking

- `UnlinkProvider` is a sensitive method under the [Identity re-authentication](identity-re-authentication.md) specification. It makes sure that the session of the access token has an open re-authentication window in the same transaction as the unlink.
- The method removes the link of the named provider from the subject of the access token. The client never names a provider subject.
- Identity allows the unlink only when the account keeps a password or a link of the other provider after it. The verified-email state does not matter. Otherwise Identity returns `FailedPrecondition` and changes nothing. A provider-only account first sets a password through the [Identity password change](identity-password-change.md) specification.
- Identity serializes `UnlinkProvider`, `SetPassword`, and provider-link changes for the same subject. 2 concurrent unlinks of the 2 providers cannot both remove the last sign-in method.
- After the unlink commits, a provider login with that provider subject no longer finds this account. It follows the new-identity and email-collision rules of the [Identity provider login](identity-provider-login.md) specification.
- After the unlink commits, `CompleteProviderReauthentication` with that provider fails for this account, because the provider subject no longer matches a link.
- A retry after a lost response, from an active session with an open re-authentication window, finds no link and returns `NotFound` without a change. The client reads `NotFound` after a lost response as a committed unlink.

### Sessions

- In the same transaction as the unlink, Identity revokes every other session of the subject whose sign-in method is the unlinked provider, and it invalidates their refresh tokens. It deletes the device data of those sessions, as the [Identity session management](identity-session-management.md) specification states.
- The session of the access token stays active, even when it signed in through the unlinked provider. Its tokens, its re-authentication window, and its device data do not change.
- A session with the `SIGN_IN_METHOD_UNSPECIFIED` method stays active, because Identity cannot tell which method started it.
- A `CheckSession` or `RefreshSession` call that starts after the commit rejects each revoked session. A request that was admitted before the commit can finish.

### Security and abuse

- `UnlinkProvider` adds no limit bucket. Re-authentication and its limits already protect the method.
- Identity sends no notice after an unlink. The maintainer accepted 2 risks: an owner does not learn that a person with a stolen session and a re-authentication removed a provider, and an account that keeps only a password with an unverified email cannot reset a forgotten password.

### Data and compatibility

- A migration adds a unique constraint on the account subject and the provider of each link. Provider login creates a link only for a new account, so no existing account has 2 links of 1 provider. The migration fails instead of deleting a link if such data exists.
- Older code stays compatible with the constraint, because older code creates a link only together with a new account and never adds a second link of a provider.
- The unlink, the revocations, and the deletion of device data commit in 1 transaction. A database failure, a timeout, or a cancellation before the commit leaves all of them unchanged.
- A rollback to the version before `UnlinkProvider` removes the method. The down migration drops the constraint and keeps each link.

### Diagnostics

Logs, traces, metrics, errors, and security events record the outcome of `UnlinkProvider`. They must never contain these values:

- Provider subjects, provider tokens, access tokens, and refresh tokens.
- Full email addresses.
- Session IDs and subject IDs in metric labels.

## Testing strategy

| Risk | Test level |
| --- | --- |
| `UnlinkProvider` removes a link outside the re-authentication window (`ID-T09`) | Integration |
| An unlink removes the last sign-in method of an account (`ID-T08`) | Unit, Integration |
| 2 concurrent unlinks, or an unlink and a password change, leave an account without a sign-in method (`ID-T12`) | Integration |
| The unlinked provider subject still signs in to or re-authenticates the account (`ID-T07`, `ID-T08`) | Integration |
| A session of the unlinked provider still refreshes or passes `CheckSession`, or keeps its device data (`ID-T13`, `ID-T21`) | Integration |
| The current session, a session of another method, or an unspecified session ends after the unlink | Integration |
| The unlink damages the password or the link of the other provider, so a kept sign-in method stops working (`ID-T08`) | Integration |
| Identity rejects a valid unlink because the account email is not verified | Integration |
| An unlink of a provider that the account never linked returns a result other than `NotFound`, or changes state (`ID-T08`) | Integration |
| A database failure, a timeout, or a cancellation leaves part of the unlink committed (`ID-T12`, `ID-T16`) | Integration |
| A retry after a lost response gives an effect other than `NotFound` | Integration |
| A malformed or unspecified provider value reaches the database (`ID-T15`) | Unit |
| The migration deletes a link, or allows 2 links of 1 provider on 1 account | Integration |
| Telemetry or errors contain a provider subject, a token, or a full email (`ID-T17`) | Integration |
| The flow fails end to end through the generated REST route and step-up | Cluster |

## Implementation boundaries

Ask the maintainer before these actions:

- Revoke sessions of other sign-in methods after an unlink.

## Success criteria

1. Given an account with a password and a Google link, and an open re-authentication window, When the session unlinks Google, Then the Google account no longer signs in to the Flowspace account, and the password still works.
2. Given an account with a Google link and a GitHub link and no password, When the session unlinks Google, Then the unlink succeeds, and GitHub still signs in.
3. Given a provider-only account with 1 link and no password, When the session unlinks that provider, Then Identity returns `FailedPrecondition` and keeps the link.
4. Given an account with a password and an unverified email, When the session unlinks its only provider, Then the unlink succeeds.
5. Given a closed re-authentication window, When the session calls `UnlinkProvider`, Then Identity changes nothing and returns `PermissionDenied` with the reason `REAUTHENTICATION_REQUIRED`.
6. Given an account without a GitHub link, When the session unlinks GitHub, Then Identity returns `NotFound` and changes nothing.
7. Given another session signed in with the unlinked provider, When the unlink commits, Then that session cannot refresh or pass `CheckSession`, and Identity deletes its device data.
8. Given the current session signed in with the unlinked provider, When the unlink commits, Then the current session refreshes and passes `CheckSession`.
9. Given another session signed in with a password or with `SIGN_IN_METHOD_UNSPECIFIED`, When the unlink commits, Then that session stays active.
10. Given an account with only a Google link and a GitHub link, When 2 sessions unlink Google and GitHub at the same time, Then at most 1 unlink succeeds, and the account keeps 1 link.
11. Given an unlinked Google account, When the user signs in with it, Then Identity does not sign in to the old Flowspace account, and it applies the new-identity rules of provider login.
12. Given an active session with an open re-authentication window, and a successful `UnlinkProvider` response is lost, When the client retries, Then Identity returns `NotFound` and changes nothing.
13. Given the unlink tests run, When a test inspects logs, traces, metrics, and errors, Then they contain no provider subject, token, or full email address.
