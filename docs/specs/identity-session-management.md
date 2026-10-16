# Spec: Identity session management

Module ID: `identity-session-management`

Status: Draft

## Objective

Let a signed-in user see each active session of their account and end 1 session that they do not recognize, without a logout of every device. The user tells the sessions apart by the sign-in method, the device, the IP address, and the times when each session started and was last used. The first users are API clients, and all data is disposable under [ADR-0022](../adr/0022-single-host-storage-holds-disposable-data.md).

The IP address and the user agent of each session are personal data. Identity deletes them when the session ends, and only the owner of the session can read them.

## Scope and ADRs

The scope covers the device data that Identity records when a session starts and when it refreshes, a list of the active sessions of the caller, and the revocation of 1 other session of the caller. It also covers the deletion of the device data when a session ends.

This capability excludes these items:

- A method that revokes every session except the current one. `LogoutAllSessions` keeps its behavior.
- An email notice when a new device signs in.
- A location or a country from an IP address.
- A history of sessions that ended.
- A name that the user gives a device.

These decisions apply:

- [ADR-0008](../adr/0008-list-methods-use-cursor-pagination.md): `ListSessions` uses a bounded page size and an opaque page token.
- [ADR-0012](../adr/0012-field-masks-and-action-rpcs-model-updates.md): `RevokeSession` is an action RPC on a session resource.
- [ADR-0013](../adr/0013-acting-identity-comes-from-the-token.md): the subject of the sessions comes from the access token, never from the request.
- [ADR-0031](../adr/0031-flowspace-owns-authentication-and-revocable-sessions.md): Identity owns sessions, and a revocation is durable before success.

## Contract

Use package `flowspace.identity.v1` and service `IdentityService`.

### Methods

| RPC | Public HTTP route | Request fields | Response fields |
| --- | --- | --- | --- |
| `ListSessions` | `GET /v1/sessions` | `page_size`, `page_token` | `sessions`, `next_page_token` |
| `RevokeSession` | `POST /v1/sessions/{session_id}:revoke` | `session_id` (required) | None |

### Method requirements

| RPC | Authentication | Retry |
| --- | --- | --- |
| `ListSessions` | Bearer access token | Safe to retry |
| `RevokeSession` | Bearer access token | Safe to retry |

### Resource fields

Session:

| Protobuf field | JSON field | Meaning |
| --- | --- | --- |
| `session_id` | `sessionId` | The ID of the session, the same value as the session ID in its access tokens, output only |
| `current` | `current` | `true` for the session of the access token of the request, output only |
| `sign_in_method` | `signInMethod` | `SIGN_IN_METHOD_PASSWORD`, `SIGN_IN_METHOD_GOOGLE`, `SIGN_IN_METHOD_GITHUB`, or `SIGN_IN_METHOD_UNSPECIFIED` for a session that started before this module, output only |
| `created_at` | `createdAt` | The server time when the session started, output only |
| `created_ip_address` | `createdIpAddress` | The trusted source IP address of the request that started the session, in IPv4 dotted or IPv6 text form, or empty when Identity does not know it, output only |
| `created_user_agent` | `createdUserAgent` | The `User-Agent` value of that request, at most 512 bytes, or empty, output only |
| `created_device` | `createdDevice` | A label that Identity derives from `created_user_agent`, such as `Chrome on macOS`, or `Unknown device`, output only |
| `last_used_at` | `lastUsedAt` | The server time of the last successful refresh, or `created_at` before the first refresh, output only |
| `last_used_ip_address` | `lastUsedIpAddress` | The trusted source IP address of that request, in the same form as `created_ip_address`, output only |
| `last_used_user_agent` | `lastUsedUserAgent` | The `User-Agent` value of that request, at most 512 bytes, or empty, output only |
| `last_used_device` | `lastUsedDevice` | A label that Identity derives from `last_used_user_agent`, output only |
| `expires_at` | `expiresAt` | The earlier of the idle expiry and the absolute expiry of the session, output only |

### Errors

| Condition | gRPC status | HTTP status |
| --- | --- | --- |
| Malformed request, page size, page token, or session ID | `InvalidArgument` | 400 |
| Missing, invalid, or inactive session of the caller | `Unauthenticated` | 401 |
| The session ID names the current session | `FailedPrecondition` | 400 |
| `RevokeSession` is called outside the re-authentication window | `PermissionDenied` | 403 |
| The session does not exist or belongs to another subject | `NotFound` | 404 |
| Identity is unavailable | `Unavailable` | 503 |

## Behavior

### Device data

- When `CreateAccount`, `ClaimUnverifiedAccount`, `CreatePasswordSession`, or `CreateProviderSession` creates a session, Identity records the sign-in method, the trusted source IP address, and the `User-Agent` value of the request in the same transaction. Signup and account claim use the password method.
- The source IP address comes only from the configured edge proxies, as in password login. Identity stores it as an IP address type, not as free text. When no trusted address exists, the field is empty.
- Identity keeps at most 512 bytes of a `User-Agent` value and cuts a longer value at the last whole UTF-8 character. It keeps an invalid UTF-8 value as empty. It never rejects a request because of its `User-Agent` value.
- Identity derives the device label from the user agent with a fixed parser of browser and operating-system names. The label has the form `<browser> on <operating system>`. A user agent that the parser does not recognize gives `Unknown device`.
- Each successful `RefreshSession` sets the last-used time, IP address, and user agent of the session in the same transaction as the token rotation, and it keeps the created values.
- A refresh that fails without a revocation, such as an unknown token or a full limit, does not change the device data. A replayed refresh token revokes its session, as the [Refresh and replay](identity-password-login-and-sessions.md#refresh-and-replay) rules state, so the revocation deletes the device data, and the list no longer shows the session.
- `CheckSession` and other requests do not change the last-used values.

### Listing

- `ListSessions` returns the sessions of the subject of the access token that are not revoked and not expired at the server time of the request. A session is expired at its exact expiry time.
- The order is `last_used_at` from newest to oldest, then `session_id` in ascending byte order. The page token holds the position in this order, and it works only for the subject that received it. `ListSessions` accepts a changed `page_size` with a page token.
- The default page size is 20, and the maximum is 100. A page size of 0 uses the default. A negative page size, or a page size above 100, is malformed.
- A session that a refresh moves or that ends between 2 pages can appear twice or be missing. The client reads the list again for an exact view.
- `ListSessions` does not need re-authentication.

### Revocation

- `RevokeSession` is a sensitive method under the [Identity re-authentication](identity-re-authentication.md) specification. It makes sure that the session of the access token has an open re-authentication window in the same transaction as the revocation.
- The target session must belong to the subject of the access token. A session ID of another subject and an unknown session ID return the same `NotFound` error with the same practical timing.
- The current session returns `FailedPrecondition`. The client uses `LogoutCurrentSession` for it.
- A successful revocation sets the revoked time of the target session, invalidates its refresh tokens, and deletes its device data in 1 transaction, before the response. A `CheckSession` or `RefreshSession` call that starts after the commit rejects the target session. A request that was admitted before the commit can finish.
- `RevokeSession` serializes with `RefreshSession` of the target session, so a refresh cannot commit a new token pair after the revocation commits.
- A target session of the caller that is already revoked or expired returns success without a change, so a retry after a lost response has no additional effect.

### Security and abuse

- Only the owner of a session can read its device data. Other services and other subjects never receive it, and `CheckSession` does not return it.
- `ListSessions` and `RevokeSession` add no limit buckets.

### Data and compatibility

- A migration adds the sign-in method, the 2 IP addresses, the 2 user agents, and the last-used time to each session. Existing sessions get no device data and the `SIGN_IN_METHOD_UNSPECIFIED` method. Their last-used time is their creation time until their next refresh.
- Each revocation deletes the device data of the revoked session in the revocation transaction. This includes `LogoutCurrentSession`, `LogoutAllSessions`, `ResetPassword`, `ClaimUnverifiedAccount`, refresh-token replay, and `RevokeSession`.
- `identity-worker` deletes the device data of each expired session within 1 hour after its expiry, because no request or job runs at the exact moment of expiry. `ListSessions` never returns an expired session, so no user reads device data after expiry.
- The device data is not part of any event, and no other service stores it.
- Older code ignores the new columns. A rollback before the cleanup runs leaves device data of expired sessions in the database until a newer version runs again.

### Diagnostics

Logs, traces, metrics, errors, and security events record the method and the outcome of `ListSessions` and `RevokeSession`. They must never contain these values:

- IP addresses and user agents from the device data, including in span attributes.
- Access tokens, refresh tokens, and page tokens.
- Session IDs and subject IDs in metric labels.

## Testing strategy

| Risk | Test level |
| --- | --- |
| A new session from signup, account claim, password login, or provider login has no device data or the wrong method (`ID-T21`) | Integration |
| An IP address from an untrusted header reaches the device data (`ID-T15`) | Unit, Integration |
| A long, malformed, or hostile user agent breaks a request or the label parser (`ID-T15`, `ID-T16`) | Unit |
| A successful refresh does not update the last-used values, or changes the created values | Integration |
| A failed refresh changes the device data, or a replayed refresh leaves the session listed or its device data stored (`ID-T11`) | Integration |
| `ListSessions` returns a revoked or expired session, or a session of another subject (`ID-T09`, `ID-T21`) | Integration |
| `ListSessions` returns a session at its exact expiry time, or the cleanup misses the bound, before, at, and after each boundary (`ID-T19`) | Unit, Integration |
| A page token works for another subject or breaks the order across pages (`ID-T15`) | Unit, Integration |
| `RevokeSession` revokes a session outside the re-authentication window (`ID-T09`) | Integration |
| A session ID of another subject gives a different result or timing from an unknown ID (`ID-T01`) | Integration |
| `RevokeSession` revokes the current session (`ID-T13`) | Unit |
| A revoked session still refreshes or passes `CheckSession` after the commit (`ID-T13`, `ID-T14`) | Integration |
| A refresh of the target session commits after its revocation (`ID-T12`) | Integration |
| A retry after a lost `RevokeSession` response gives an error or a second effect | Integration |
| A revocation by any method, or an expiry, leaves device data after the stated time (`ID-T21`) | Integration |
| Telemetry or errors contain an IP address, a user agent, or a token (`ID-T17`) | Integration |
| The list and revoke flow fails end to end through the generated REST routes and step-up | Cluster |

## Implementation boundaries

Ask the maintainer before these actions:

- Keep the device data longer than 1 hour after expiry, or keep it after a revocation.
- Add a location, an IP address range, or another field from the device data.
- Return device data to another service or another subject.

## Success criteria

1. Given a user signs in with a password, Google, or GitHub, When the user lists sessions, Then the new session shows the sign-in method, the IP address, the user agent, the device label, and `current=true`.
2. Given a session refreshes from another IP address and user agent, When the user lists sessions, Then the session shows the new last-used values and keeps its created values.
3. Given a refresh fails with an unknown token, When the user lists sessions, Then the last-used values do not change.
4. Given a session replays a used refresh token, When the user lists sessions, Then the list no longer contains that session, and Identity deletes its device data.
5. Given a subject has a revoked session and an expired session, When the user lists sessions, Then the list contains neither of them.
6. Given another subject has sessions, When the user lists sessions, Then the list contains only sessions of the caller.
7. Given a subject has 25 active sessions and no session changes during the read, When the user lists with the default page size and then the next page token, Then the 2 pages contain all 25 sessions in the stated order without duplicates.
8. Given a page token from 1 subject, When another subject sends it, Then Identity returns `InvalidArgument`.
9. Given an open re-authentication window, When the user revokes another session of the account, Then that session cannot refresh or pass `CheckSession`, and Identity deletes its device data.
10. Given a closed re-authentication window, When the user calls `RevokeSession`, Then Identity changes nothing and returns `PermissionDenied` with the reason `REAUTHENTICATION_REQUIRED`.
11. Given the session ID of the current session, When the user calls `RevokeSession`, Then Identity returns `FailedPrecondition` and the session stays active.
12. Given a session ID of another subject or an unknown session ID, When the user calls `RevokeSession`, Then Identity returns the same `NotFound` error and changes no session.
13. Given an active session with an open re-authentication window, and a successful `RevokeSession` response is lost, When the client retries, Then Identity returns success and changes nothing.
14. Given a session ends through `LogoutCurrentSession`, `LogoutAllSessions`, or `ResetPassword`, When the transaction commits, Then Identity deletes the device data of that session.
15. Given a session expires, When 1 hour passes, Then `identity-worker` deletes its device data.
16. Given a `User-Agent` value of more than 512 bytes or with invalid UTF-8, When a session starts, Then the request succeeds, and Identity stores at most 512 bytes of valid UTF-8 or an empty value.
17. Given the session-management tests run, When a test inspects logs, traces, metrics, and errors, Then they contain no IP address, user agent, or token.
