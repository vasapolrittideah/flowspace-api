# Spec: Identity email change

Module ID: `identity-email-change`

Status: Draft

## Objective

Let a signed-in user change the email address of their account. The user proves control of the new address with a 6-digit code, and the account then uses the new address for sign-in, recovery, and notices. A user who typed a wrong address at signup can also fix it this way. The first users are API clients, and all data is disposable under [ADR-0022](../adr/0022-single-host-storage-holds-disposable-data.md).

The change does not reveal whether another account uses the new address. When the old address is verified, Identity tells its owner about the change.

## Scope and ADRs

The scope covers the request for a change, the code to the new address, the confirmation of the change, and the notice to the old address.

This capability excludes these items:

- A confirmation from the old address.
- An undo of a change, or a recovery of an account that someone took over.
- A hold on the new address while the change waits for its code.
- More than 1 email address on an account.

These decisions apply:

- [ADR-0013](../adr/0013-acting-identity-comes-from-the-token.md): the account comes from the access token, never from the request.
- [ADR-0031](../adr/0031-flowspace-owns-authentication-and-revocable-sessions.md): Identity owns email addresses and their verified state, and it uses 6-digit codes by email.
- [ADR-0035](../adr/0035-identity-signup-security-and-mail-delivery.md): codes and notices use the durable mail delivery of signup.
- [ADR-0047](../adr/0047-email-change-codes-go-to-the-pending-address.md): the mail worker sends an email-change code only to the new address of the current email-change challenge.

## Contract

Use package `flowspace.identity.v1` and service `IdentityService`.

The `new_email` field follows the email rules of the [Identity signup and email verification](identity-signup-and-email-verification.md) specification. The `code` field is a string of exactly 6 ASCII digits, and leading zeroes are significant.

### Methods

| RPC | Public HTTP route | Request fields | Response fields |
| --- | --- | --- | --- |
| `RequestEmailChange` | `POST /v1/email-change-requests` | `new_email` (required) | `accepted` |
| `ConfirmEmailChange` | `POST /v1/email-changes` | `code` (required) | `email_verified` |

### Method requirements

| RPC | Authentication | Retry |
| --- | --- | --- |
| `RequestEmailChange` | Bearer access token | Safe to retry |
| `ConfirmEmailChange` | Bearer access token | Safe to retry |

### Resource fields

Email change result:

| Protobuf field | JSON field | Meaning |
| --- | --- | --- |
| `accepted` | `accepted` | `true` when Identity accepted the request, whether or not it sent a code, output only |
| `email_verified` | `emailVerified` | `true` after a confirmed change, output only |

### Published events

| Event | Topic | Fields |
| --- | --- | --- |
| `EmailDeliveryRequested` | `identity.email-delivery.v1` | `event_id`, `challenge_id`, `purpose` |

### Errors

| Condition | gRPC status | HTTP status |
| --- | --- | --- |
| Malformed request, malformed email, malformed code, or a new email equal to the current email | `InvalidArgument` | 400 |
| Wrong, expired, or used code, or a new email that another account took before the confirmation | `InvalidArgument` | 400 |
| Missing, invalid, or inactive session | `Unauthenticated` | 401 |
| A client calls `RequestEmailChange` or `ConfirmEmailChange` outside the re-authentication window | `PermissionDenied` | 403 |
| A limit is reached | `ResourceExhausted` | 429 |
| Identity, its database, or its limit store is unavailable | `Unavailable` | 503 |

## Behavior

### Requesting a change

- `RequestEmailChange` and `ConfirmEmailChange` are sensitive methods under the [Identity re-authentication](identity-re-authentication.md) specification. `RequestEmailChange` makes sure that the session of the access token has an open re-authentication window before it creates a code.
- Identity compares the new email with the current email by the comparison rules of the signup specification. An equal address returns `InvalidArgument`.
- When no other account uses the new email, Identity creates a 6-digit challenge for the subject, the new email, and the email-change purpose. It creates a durable delivery request in the same transaction. The `purpose` field of `EmailDeliveryRequested` is `change-email`. Identity sends the code only to the new email, and the mail worker discards the delivery when the challenge is replaced, consumed, or expired.
- When another account uses the new email, verified or not, Identity creates no challenge and sends no code. It returns the same `accepted=true` response with the same practical timing.
- A new request replaces the previous email-change challenge of the subject. A request within 60 seconds of the previous request returns `accepted=true` and does not replace the challenge.
- Identity holds no address for the subject while the challenge waits. Another person can sign up with the new email before the confirmation.
- A request changes no email, verified state, link, or session.

### Confirming a change

- `ConfirmEmailChange` makes sure that the re-authentication window is open in the same transaction as the change.
- The code expires 10 minutes after Identity issues it and succeeds once. An email-change code cannot verify an email, claim an account, reset a password, or re-authenticate a session. A code of another purpose cannot change an email.
- In the same transaction, Identity makes sure again that no other account uses the new email. If another account took it after the request, Identity consumes the code, changes nothing, and returns the same error as a wrong code.
- A successful confirmation sets the account email to the new address and sets `email_verified=true`. It invalidates every outstanding verification, claim, reset, and re-authentication code of the old address.
- The old address is free at once. Another person can sign up with it after the commit.
- The confirmation issues no token, revokes no session, and changes no password or provider link. Provider login continues to find the account by its provider subject.
- After the commit, password login, password recovery, re-authentication codes, and notices use the new address. A `CheckSession` that starts after the commit returns `email_verified=true`.
- A retry after a lost response returns the code error, and the change that the first request made stays in place.

### Notice to the old address

- When the old address was verified at the time of the change, Identity queues a durable email-change notice to the old address in the same transaction. The notice states that the account email changed. It contains no code, no full new address, and no link to undo the change.
- When the old address was not verified, Identity queues no notice.
- If delivery fails after the commit, Identity retries the notice without a change to the account.

### Unverified accounts

- An account with an unverified email can change it. The new address becomes the verified address when the change commits.
- Such an account re-authenticates with a password or a linked provider, because a re-authentication code needs a verified email.

### Security and abuse

- `RequestEmailChange` counts against the code-request limits and `ConfirmEmailChange` against the wrong-guess limits that [Email code and verification](identity-signup-and-email-verification.md#email-code-and-verification) and [Abuse and failure handling](identity-signup-and-email-verification.md#abuse-and-failure-handling) share across verification, claim, reset, and re-authentication codes. It uses those buckets, not new ones.
- A request for a used address counts against the limits like any other request.
- All limits use shared state across replicas, count retries, and fail closed with `Unavailable` when the limit store is unavailable.
- Identity serializes `ConfirmEmailChange` with signup, account claim, and other email changes, so that 2 accounts cannot hold 1 address. The unique rule on account email addresses decides concurrent commits.
- A person who has an active session and the mailbox of the account can re-authenticate and move the account to their own address. The maintainer accepted this risk. The verified old address receives a notice.

### Data and compatibility

- A migration adds the email-change purpose to email challenges, the new address to such a challenge, and an email-change notice kind to the durable notice queue.
- The email change, the code consumption, the invalidation of old codes, and the notice commit in 1 transaction. A database failure, a timeout, or a cancellation before the commit leaves all of them unchanged.
- Older code ignores the new purpose and notice kind. Before a rollback to the version before `RequestEmailChange`, delete the pending email-change challenges and notices. The changed email addresses stay in place.

### Diagnostics

Logs, traces, metrics, errors, and security events record the outcome of `RequestEmailChange` and `ConfirmEmailChange`. They must never contain these values:

- Codes, access tokens, and refresh tokens.
- Full email addresses, old or new.
- Whether another account uses the new address.
- Session IDs and subject IDs in metric labels.

## Testing strategy

| Risk | Test level |
| --- | --- |
| A request or a confirmation succeeds outside the re-authentication window (`ID-T09`) | Integration |
| A response, its timing, or the limits reveal that another account uses the new address (`ID-T01`) | Integration |
| A code of another purpose changes the email, or an email-change code verifies, claims, resets, or re-authenticates (`ID-T03`) | Unit, Integration |
| Code requests or guesses bypass the shared code limits across replicas (`ID-T03`, `ID-T16`) | Integration |
| A code works at or after its expiry, or twice (`ID-T04`, `ID-T19`) | Unit, Integration |
| Identity accepts a new address equal to the current address under the signup comparison rules | Unit, Integration |
| A code of another subject or of a replaced challenge changes the email, or a repeat request within 60 seconds replaces the challenge (`ID-T03`) | Integration |
| A queued delivery of a replaced, consumed, or expired challenge reaches an address (`ID-T03`) | Integration |
| A failed delivery of a code or a notice creates a second valid code, changes the account, or never retries | Integration |
| A retry after a lost confirmation response makes a second change or queues a second notice (`ID-T12`) | Integration |
| Concurrent confirmations, signups, or claims give 1 address to 2 accounts (`ID-T12`) | Integration |
| An address that another account took after the request moves to this account | Integration |
| An old-address code still works after the change (`ID-T03`) | Integration |
| The old address stays held after the change, so a signup with it fails | Integration |
| The change revokes a session, issues a token, or changes a password or provider link | Integration |
| Password login or password recovery still uses the old address after the change | Integration |
| A notice is missing for a verified old address, is queued for an unverified one, or contains a code or the new address (`ID-T04`, `ID-T17`) | Integration |
| An unverified account cannot change its email, or stays unverified after the change | Integration |
| A database failure, a timeout, or a cancellation leaves part of the change committed (`ID-T12`, `ID-T16`) | Integration |
| A malformed email or code reaches the database (`ID-T15`) | Unit |
| Telemetry or errors contain a code, a token, or a full email (`ID-T17`) | Integration |
| The flow fails end to end through the generated REST routes, step-up, and Mailpit | Cluster |

## Implementation boundaries

Ask the maintainer before these actions:

- Add a link or a code to the notice that lets the old address undo the change.
- Hold the new address for the subject while the change waits.

## Success criteria

1. Given an open re-authentication window and an unused new address, When the session requests a change and confirms the code from the new address, Then the account email is the new address, and `email_verified=true`.
2. Given a closed re-authentication window, When the session calls `RequestEmailChange` or `ConfirmEmailChange`, Then Identity changes nothing and returns `PermissionDenied` with the reason `REAUTHENTICATION_REQUIRED`.
3. Given a new address that another account uses, When the session requests a change, Then Identity returns `accepted=true`, sends no code, and changes nothing.
4. Given a pending change, When another person signs up with the new address before the confirmation, Then the confirmation returns the code error and changes nothing.
5. Given a new address equal to the current address under the signup comparison rules, When the session requests a change, Then Identity returns `InvalidArgument`.
6. Given a verified old address, When the change commits, Then Identity queues 1 email-change notice to the old address without a code or the full new address.
7. Given an unverified old address, When the change commits, Then the account is verified, and Identity queues no notice.
8. Given a completed change, When the user logs in with the new address and the same password, Then the login succeeds, and a login with the old address fails.
9. Given a completed change, When the user requests a password reset for the new address, Then Identity sends the reset code to the new address.
10. Given outstanding verification, claim, reset, or re-authentication codes of the old address, When the change commits, Then none of them works.
11. Given a completed change, When another person signs up with the old address, Then the signup succeeds.
12. Given a session of the account, When the change commits, Then the session stays active, and its next `CheckSession` returns `email_verified=true`.
13. Given a verification, claim, reset, or re-authentication code, When the session submits it to `ConfirmEmailChange`, Then Identity returns the code error and changes nothing.
14. Given 2 accounts confirm changes to the same address at the same time, When both commit, Then at most 1 account gets the address.
15. Given an email-change limit is full, When the session requests a change or confirms a code, Then Identity returns `ResourceExhausted` without a code or a change.
16. Given the email-change tests run, When a test inspects logs, traces, metrics, and errors, Then they contain no code, token, or full email address.
