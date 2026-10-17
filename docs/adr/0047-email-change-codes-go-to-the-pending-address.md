# ADR-0047: Email-change codes go to the pending address

Date: 2026-10-17

Status: Accepted; scoped exception to [ADR-0035](0035-identity-signup-security-and-mail-delivery.md)

## Context

A signed-in user will change the email address of their account. The user must prove control of the new address before the account uses it. As a result, the 6-digit code must reach the new address, not the current one.

[ADR-0035](0035-identity-signup-security-and-mail-delivery.md) protects queued email codes. The mail worker locks the account and the current challenge, makes sure that the challenge is valid, and sends only if the decrypted recipient matches the current account address. This rule stops a replaced or stale delivery from reaching an old address. Under this rule, the worker discards every code for a new address, because the new address is not the current account address until the change commits.

## Decision

For the email-change purpose only, the mail worker compares the decrypted recipient with the new address of the current email-change challenge of the subject, not with the current account address. Every other purpose keeps the recipient rule of ADR-0035.

The email-change challenge stores its new address with the subject and the purpose. The worker locks the account and that challenge in a database transaction. It makes sure that the challenge ID, the purpose, the account state, and the 10-minute expiry meet the rules of ADR-0035, then decrypts the payload. It sends only if the decrypted recipient matches the new address of that challenge. It discards a replaced, consumed, or expired challenge without sending.

The other controls of ADR-0035 apply without change: the keyed one-way verifier, the AES-256-GCM payload with the challenge ID, purpose, and subject as associated data, the opaque broker record, the lock through the mail call, and the deletion of delivery material after acceptance, replacement, consumption, and expiry.

The notice that tells the old address about a completed change is not a code. Like the password-change notice, it stores its own recipient in the durable notice queue, and the recipient rule of ADR-0035 does not apply to it.

## Alternatives Considered

### A code to the current address

- Pros: the worker keeps the recipient rule of ADR-0035 for every purpose.
- Cons: the code does not prove control of the new address, and a typo in the new address locks the account to an address that nobody controls.
- Rejected: only a code at the new address proves that the user controls it.

### A pending address on the account

- Pros: the worker can treat the pending address as a current address and keep 1 recipient lookup.
- Cons: the account holds an unproved address, and every other reader of the account address must learn to ignore it.
- Rejected: the new address belongs to the challenge until the change commits, so no reader of the account sees an unproved address.

### No recipient check for email-change codes

- Pros: the worker needs no new lookup, and the challenge-ID check still discards replaced challenges.
- Cons: the worker no longer makes sure that the decrypted recipient is the address that the challenge stores, so a payload written with a wrong address still reaches that address.
- Rejected: the comparison with the stored new address keeps the independent recipient check of ADR-0035 for email-change codes.

## Consequences

- The mail worker has 2 recipient rules: the current account address for verification, claim, reset, and re-authentication codes, and the challenge address for email-change codes.
- An email-change request reaches an address that the account does not own yet. The per-account and per-source code limits bound how many such emails 1 caller can send.
- The Identity email change specification defines the request, the confirmation, the notice, the limits, and the tests.
