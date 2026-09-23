# ADR-0032: Identity signup recovers with login

## Status

Proposed

## Date

2026-09-23

## Context

[ADR-0011](0011-idempotency-keys-protect-non-idempotent-creates.md) requires an `Idempotency-Key` for non-idempotent creates and replays the original result. Its key is scoped to an authenticated subject. Identity signup has no authenticated subject and returns a refresh token that Identity stores only as a hash under [ADR-0031](0031-flowspace-owns-authentication-and-revocable-sessions.md). Identity cannot replay the original token without retaining a reusable secret.

The first clients use disposable data. Before real users join, Identity will support password login. A user who loses a signup response still knows the email address and password used for signup. An email owner who does not know the password of an unverified account needs a separate way to take control of that address.

## Decision

`CreateAccount` and `ClaimUnverifiedAccount` do not accept `Idempotency-Key`. Identity rejects that header on these methods so clients do not assume exact response replay. Other protected creates continue to follow ADR-0011.

Identity enforces the documented email uniqueness rule in one database transaction. If a client repeats `CreateAccount` after a lost response, the retry creates an account if the first attempt did not commit. If the first attempt committed, the retry returns `AlreadyExists` without tokens. The user recovers the session through password login after that capability exists. Until login exists, this failure case is limited to disposable data.

If an email owner does not know the password of a pending unverified account, Identity offers a separate email-code claim. A valid claim creates a new subject and verified account, revokes the old account's sessions, and issues one new session. A lost claim response is also recovered through password login. The [signup and email verification spec](../specs/identity-signup-and-email-verification.md) defines the public contract and limits.

## Alternatives Considered

### Store the original refresh token for replay

- Pros: A retry with the same key could receive the original response.
- Cons: Identity would retain a reusable refresh token that could be exposed by a storage breach.
- Rejected: The token model stores only refresh-token hashes.

### Issue replacement tokens for an idempotency-key replay

- Pros: The client could recover without a separate login call.
- Cons: The response would not be the original completed result required by ADR-0011. It would also create or rotate session state during a retry.
- Rejected: A login call gives the same recovery without changing the idempotency contract.

### Claim the account after every lost signup response

- Pros: One recovery path could handle both lost responses and unknown passwords.
- Cons: A user who knows the signup password would replace a stable subject and need another email code.
- Rejected: Password login preserves the existing account for that case.

## Consequences

- A duplicate signup returns `AlreadyExists` (HTTP 409) without tokens. Clients must not expect `CreateAccount` or `ClaimUnverifiedAccount` to replay tokens after an uncertain result.
- Email uniqueness, one-time claim codes, rate limits, and atomic account replacement protect the exception. A successful claim retires the old subject and every old session.
- Password login must exist before real users can recover a lost signup or claim response. Other creates keep the `Idempotency-Key` contract from ADR-0011.
- This record is a scoped exception to ADR-0011. It does not change that record's rule for other operations.
