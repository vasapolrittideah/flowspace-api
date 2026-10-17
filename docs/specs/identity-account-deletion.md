# Spec: Identity account deletion

Module ID: `identity-account-deletion`

Status: Draft

## Objective

Let a signed-in user delete their Flowspace account, and let them change their mind for 30 days. During that grace period the account cannot use Flowspace, but the user can sign in and restore it. At the end of the grace period, Identity deletes the personal data of the account and keeps only its subject. The first users are API clients, and all data is disposable under [ADR-0022](../adr/0022-single-host-storage-holds-disposable-data.md).

Other services learn about each step from events, and each service decides its own effects. A user can always delete their account, also when the account is the only owner of a workspace.

## Scope and ADRs

The scope covers the deletion request, the grace period with its restricted sessions, the restore, the final deletion, the notices, and the account lifecycle events.

This capability excludes these items:

- A restore after the final deletion, or from a backup.
- An export of the account data.
- The effects in Workspace or another service. Each service specifies its own effects.
- A deletion by an administrator.

These decisions apply:

- [ADR-0013](../adr/0013-acting-identity-comes-from-the-token.md): the account comes from the access token, never from the request.
- [ADR-0032](../adr/0032-identity-signup-recovers-with-login.md): the email owner can claim an unverified account, also during its grace period.
- [ADR-0031](../adr/0031-flowspace-owns-authentication-and-revocable-sessions.md): Identity owns accounts and sessions, and a revocation is durable before success.
- [ADR-0048](../adr/0048-identity-publishes-account-lifecycle-events.md): Identity publishes the account lifecycle events with the subject as the only account reference.

## Contract

Use package `flowspace.identity.v1` and service `IdentityService`.

Each time field uses `google.protobuf.Timestamp`, and JSON shows it as an RFC 3339 string in UTC.

The `DeleteAccount` response has 1 field. `deletion_due_at` (JSON `deletionDueAt`) is the due time, 30 days after the request, output only. After the due time, the user can no longer restore the account, and the final deletion follows within 1 hour.

The `CreatePasswordSession`, `CreateProviderSession`, and `RefreshSession` responses get 1 new optional field. `account_deletion_due_at` (JSON `accountDeletionDueAt`) is the due time for a restricted session, output only. For a normal session, the field is unset, and JSON omits it.

### Methods

| RPC | Public HTTP route | Request fields | Response fields |
| --- | --- | --- | --- |
| `DeleteAccount` | `POST /v1/account-deletions` | None | `deletion_due_at` |
| `RestoreAccount` | `POST /v1/account-restorations` | None | None |

### Method requirements

| RPC | Authentication | Retry |
| --- | --- | --- |
| `DeleteAccount` | Bearer access token | Safe to retry |
| `RestoreAccount` | Bearer access token | Safe to retry |

### Published events

| Event | Topic | Fields |
| --- | --- | --- |
| `AccountDeletionScheduled` | `identity.account-lifecycle.v1` | `event_id`, `subject`, `occurred_at`, `deletion_due_at` |
| `AccountRestored` | `identity.account-lifecycle.v1` | `event_id`, `subject`, `occurred_at` |
| `AccountDeleted` | `identity.account-lifecycle.v1` | `event_id`, `subject`, `occurred_at` |

### Errors

| Condition | gRPC status | HTTP status |
| --- | --- | --- |
| Malformed request | `InvalidArgument` | 400 |
| Missing, invalid, or inactive session | `Unauthenticated` | 401 |
| A client calls `DeleteAccount` outside the re-authentication window | `PermissionDenied` | 403 |
| A restricted session calls a method other than `RestoreAccount`, `RefreshSession`, or `LogoutCurrentSession` | `FailedPrecondition` | 400 |
| Identity or its database is unavailable | `Unavailable` | 503 |

The `FailedPrecondition` error for a restricted session carries an `ErrorInfo` detail whose reason is `ACCOUNT_DELETION_PENDING` and whose domain is `identity.flowspace`.

## Behavior

### Requesting a deletion

- `DeleteAccount` is a sensitive method under the [Identity re-authentication](identity-re-authentication.md) specification. It makes sure that the session of the access token has an open re-authentication window in the same transaction as the request.
- In 1 transaction, Identity records the request time, sets the due time to 30 days later, revokes every session of the subject including the current one, invalidates their refresh tokens, deletes their device data, and queues an `AccountDeletionScheduled` event.
- Identity does not ask another service before a deletion. Identity can delete an account that owns a workspace alone.
- A retry after a lost response finds the current session revoked and returns `Unauthenticated` without a second change.

### Grace period

- During the 30 days, the account keeps its email address, password, provider links, and verified state. Another person cannot sign up with the email or link the provider subjects.
- `CreatePasswordSession` and `CreateProviderSession` work as usual and create a restricted session. Their responses set `account_deletion_due_at`. A restricted session can call only `RestoreAccount`, `RefreshSession`, and `LogoutCurrentSession`. Every other Identity method returns `FailedPrecondition` with the reason `ACCOUNT_DELETION_PENDING`.
- `CheckSession` treats a restricted session as inactive and returns `Unauthenticated`, so Workspace and other services deny it.
- `RequestPasswordResetCode` and `ResetPassword` continue to work under their existing rules, so a user who forgot the password can still sign in and restore. Only an account with a verified email and a password is eligible, and a pending deletion makes no other account eligible. A reset does not restore the account.
- Identity never restores an account during a login. Only `RestoreAccount` restores it.

### Restoring an account

- `RestoreAccount` needs a restricted session of the account, which a login proves. It is not a sensitive method and needs no open re-authentication window. This is a scoped exception to the account rule of the [Identity re-authentication](identity-re-authentication.md) specification, because a restricted session can call no re-authentication method.
- In 1 transaction, Identity makes sure that the due time has not passed, clears the deletion request, and queues an `AccountRestored` event. Each restricted session of the subject that is still active becomes a normal session. A session that a logout or a password reset revoked, or that expired, stays ended.
- A `RestoreAccount` call from a normal session, such as a retry after a lost response, returns success without a change.

### Due time

- At the due time, the grace period ends, even when the final deletion has not run yet. Identity compares the due time with the server time in the transaction of each method.
- From the due time, every session of the subject is inactive. `RestoreAccount`, `RefreshSession`, `LogoutCurrentSession`, and `CheckSession` return `Unauthenticated`, as for a revoked session.
- From the due time, `CreatePasswordSession` fails with the same `Unauthenticated` error as for an unknown account. `CreateProviderSession` fails with the same `FailedPrecondition` error, message, and timing as for a provider email that belongs to another account, under the [Identity provider login](identity-provider-login.md) specification. Identity decides by the server time when the login transaction commits, also for a provider attempt that started before the due time.

### Final deletion

- `identity-worker` finds each account whose due time has passed and deletes it within 1 hour after that time. It deletes only an account that still has its deletion request in the deletion transaction, so a restore that committed first stops the deletion.
- In 1 transaction, Identity deletes the email address, the password hash, the provider links, every session and refresh-token record, every challenge, every queued notice, every provider attempt, and all device data of the subject. It keeps the subject and the deletion time as a tombstone and queues an `AccountDeleted` event.
- After the commit, the email address and the provider subjects are free. A new signup or provider login with them creates a new account with a new subject.
- Identity keeps a tombstone forever and never gives its subject to another account.

### Claiming an unverified account

- During the grace period, an account with an unverified email keeps the claim flow of the [Identity signup and email verification](identity-signup-and-email-verification.md) specification. This is the only case where the email becomes free before the due time. Without it, a person who signs up with the email of another person can hold that email for 30 days with a deletion.
- `ClaimUnverifiedAccount` retires the old subject and creates a new account as usual. In the same transaction, Identity applies the final deletion to the old subject, makes it a tombstone, and queues an `AccountDeleted` event for it. The new account has no deletion request.

### Competing changes

- A login, a refresh, a restore, a claim, a deletion request, and a final deletion of 1 account run 1 at a time, and each observes the account state that the previous one committed.
- A login that commits after a deletion request creates a restricted session. A login that commits after a restore creates a normal session.
- A restore that commits after the due time fails, and a final deletion that starts after a restore committed does nothing.

### Notices

- When the account email is verified, the deletion request queues a durable deletion notice, and the restore queues a durable restore notice, in the same transaction as their change. A notice contains the due time or the restore time, and no code or link.
- When the account email is not verified, Identity queues no notice.
- The final deletion queues no notice, because the email address no longer exists after it.

### Events

- Each event commits with its account change in 1 transaction through the Identity outbox, with the subject as the record key, under ADR-0048. The `occurred_at` field is the commit time of the change.
- `deletion_due_at` in `AccountDeletionScheduled` equals the due time in Identity. The `occurred_at` field of `AccountDeleted` is the time of the final deletion, not the due time.
- Consumers receive the events of 1 subject in commit order. A restore followed by a new deletion request produces `AccountDeletionScheduled`, `AccountRestored`, and `AccountDeletionScheduled` in that order.
- Identity never waits for a consumer.

### Security and abuse

- `DeleteAccount` adds no limit bucket. Re-authentication and its limits already protect it.
- The existing login limits apply to logins that create restricted sessions.
- A person who has an active session and can re-authenticate can schedule a deletion. The owner can restore the account during the grace period, and a verified email receives a notice.

### Data and compatibility

- A migration adds the deletion request time, the due time, and the deletion time to accounts. It lets the email columns be empty only for a tombstone, adds a restricted flag to sessions, adds 2 notice kinds, and adds the outbox records for the account lifecycle topic. A provider-only account keeps an empty password, as today.
- Each request, restore, and final deletion commits in 1 transaction with its event. A database failure, a timeout, or a cancellation before the commit leaves the account, its sessions, its notices, and its events unchanged.
- Older code never reaches a tombstone, because a tombstone has no email, session, or link. Before a rollback to the version before `DeleteAccount`, restore every account in its grace period, because older code does not know restricted sessions.

### Diagnostics

Logs, traces, metrics, errors, and security events record the outcome of each request, restore, and final deletion. They must never contain these values:

- Full email addresses, password hashes, provider subjects, and tokens.
- Session IDs and subject IDs in metric labels.

## Testing strategy

| Risk | Test level |
| --- | --- |
| `DeleteAccount` succeeds outside the re-authentication window (`ID-T09`) | Integration |
| A session survives the deletion request, or its device data stays (`ID-T13`, `ID-T21`) | Integration |
| A restricted session passes `CheckSession` or calls a method other than restore, refresh, or logout (`ID-T09`, `ID-T14`) | Integration |
| A login during the grace period restores the account without `RestoreAccount` | Integration |
| A signup or a provider link takes the email or a provider subject during the grace period (`ID-T08`) | Integration |
| A password reset during the grace period restores the account, fails for an eligible account, or makes an unverified or provider-only account eligible | Integration |
| A restore misses an active restricted session, revives an ended session, or gives a second effect on retry (`ID-T13`) | Integration |
| A restore, a refresh, or a login succeeds at or after the due time while the final deletion has not run, before, at, and after the boundary, also for a provider attempt that started before it (`ID-T19`) | Unit, Integration |
| The final deletion runs before its time, misses the 1-hour bound, or runs twice (`ID-T19`) | Unit, Integration |
| A deletion request races a login, a restore races the final deletion, or a restore races a new login, and the result differs from the serialized order (`ID-T12`) | Integration |
| A claim during the grace period leaves the old subject with personal data, without an `AccountDeleted` event, or with a deletion request on the new account (`ID-T12`, `ID-T21`) | Integration |
| The final deletion leaves personal data, a session, a challenge, a notice, or a link (`ID-T21`) | Integration |
| A tombstone subject goes to another account, or the freed email signs in to the old subject (`ID-T08`) | Integration |
| An event is missing, duplicated with a new event ID, out of order for 1 subject, or contains personal data (`ID-T17`) | Integration |
| A notice is missing for a verified email, is queued for an unverified one, or contains a code or a link (`ID-T04`) | Integration |
| A database failure, a timeout, or a cancellation leaves a change without its event, or an event without its change (`ID-T12`) | Integration |
| Telemetry or errors contain an email, a hash, a provider subject, or a token (`ID-T17`) | Integration |
| The flow fails end to end through the generated REST routes, step-up, login, restore, and the worker | Cluster |

## Implementation boundaries

Ask the maintainer before these actions:

- Change the 30-day grace period or the 1-hour bound of the final deletion.
- Keep any personal field in a tombstone.
- Let a restricted session call another method.

## Success criteria

1. Given an open re-authentication window, When the session calls `DeleteAccount`, Then Identity returns a `deletion_due_at` 30 days later, revokes every session of the subject, and queues 1 `AccountDeletionScheduled` event.
2. Given a closed re-authentication window, When the session calls `DeleteAccount`, Then Identity changes nothing and returns `PermissionDenied` with the reason `REAUTHENTICATION_REQUIRED`.
3. Given an account in its grace period, When the user signs in with the password or a linked provider, Then the response sets `account_deletion_due_at`, and the session is restricted.
4. Given a restricted session, When a protected service calls `CheckSession`, Then Identity returns `Unauthenticated`.
5. Given a restricted session, When it calls a method other than `RestoreAccount`, `RefreshSession`, or `LogoutCurrentSession`, Then Identity returns `FailedPrecondition` with the reason `ACCOUNT_DELETION_PENDING`.
6. Given a restricted session and a second restricted session that a logout ended, When the first calls `RestoreAccount`, Then the account is active, the first session passes `CheckSession`, the ended session still fails, and Identity queues 1 `AccountRestored` event.
7. Given a restored account and a lost response, When the client retries `RestoreAccount`, Then Identity returns success and queues no second event.
8. Given an account in its grace period, When another person signs up with its email, Then the signup fails as for any used email.
9. Given an account in its grace period with a verified email and a password, When the user resets the password, Then the reset succeeds, and the account stays in its grace period.
10. Given an account whose due time passed, When 1 hour passes, Then Identity keeps only the subject and the deletion time, and queues 1 `AccountDeleted` event.
11. Given a deleted account, When a new user signs up with its old email, Then the signup creates a new account with a new subject.
12. Given a deleted account with a former Google link, When a user signs in with that Google account, Then Identity applies the new-identity rules of provider login, and the old subject stays a tombstone.
13. Given a verified email, When the deletion request and a later restore commit, Then Identity queues 1 deletion notice and 1 restore notice without a code or a link.
14. Given an unverified email, When the deletion request commits, Then Identity queues no notice.
15. Given a restore followed by a new deletion request, When a consumer reads the topic, Then it receives the 3 events of the subject in commit order, each without personal data.
16. Given a restricted session and a passed due time before the final deletion runs, When the session calls `RestoreAccount`, Then Identity returns `Unauthenticated` and changes nothing.
17. Given an unverified account in its grace period, When the email owner claims it, Then the old subject is a tombstone with 1 `AccountDeleted` event, and the new account has no deletion request.
18. Given the deletion tests run, When a test inspects logs, traces, metrics, and errors, Then they contain no email address, hash, provider subject, or token.
