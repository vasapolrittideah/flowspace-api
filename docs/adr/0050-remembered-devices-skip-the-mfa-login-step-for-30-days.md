# ADR-0050: Remembered devices skip the MFA login step for 30 days

Date: 2026-10-19

Status: Accepted; scoped exception to [ADR-0049](0049-opt-in-totp-and-recovery-codes-protect-identity-accounts.md)

## Context

[ADR-0049](0049-opt-in-totp-and-recovery-codes-protect-identity-accounts.md) requires a TOTP code or a recovery code for every password login and provider login of an account with multi-factor authentication (MFA). A user who signs in often from the same device types a code each time. ADR-0049 leaves a remembered device that skips the second factor to a later record.

The [Identity session management](../specs/identity-session-management.md) specification deletes the device data of a session when the session ends. Identity therefore has no record that proves a later request comes from a device that passed MFA before.

## Decision

After a login passes the second factor, the client can ask Identity to remember the device. Identity returns a remembered-device token, and a later login of the same account that presents the token skips the MFA step. This is a scoped exception to the rule of ADR-0049 that every login of an account with MFA needs a second step.

The token has 256 random bits, and Identity stores only a hash of it with the subject and its expiry. It holds no IP address, user agent, or session ID. The token expires 30 days after the login that created it, and a use does not extend it.

The token works with password login and provider login. When the token is missing, unknown, expired, revoked, or belongs to another account, the login returns the MFA challenge as usual, not an error.

A login that skips the MFA step does not open the re-authentication window. Before a sensitive method, the user re-authenticates with a TOTP code or a recovery code, as ADR-0049 states.

These changes revoke every remembered device of the account: a password change, a password reset, turning MFA off, a new set of recovery codes, an account deletion request, and the final deletion. A logout, a provider unlink, and an email change revoke no remembered device.

A sensitive method revokes every remembered device of the account at once. Identity keeps no list of single devices, and the MFA status returns only the number of unexpired remembered devices.

## Alternatives Considered

### Recognize the device from its network and user agent

- Pros: the client stores nothing, and Identity can reuse the known devices of the new-device alert.
- Cons: a person on the same network with the same browser family looks like the owner and skips MFA with only the password.
- Rejected: a random token on the device is a real second factor, while a network and a browser family are not.

### Extend the token on each use

- Pros: a device that the user signs in from often never asks for a code again.
- Cons: a stolen token stays valid as long as the thief uses it, and the device never proves MFA again.
- Rejected: a fixed 30-day life makes each device prove the second factor again at least once a month.

### A list of remembered devices with revocation of 1 device

- Pros: a user can end 1 lost device and keep the others.
- Cons: Identity needs a list method, a revoke method for 1 device, and a name for each device that the user can recognize, and each client must show the list.
- Rejected: revoking all devices covers a lost device with 1 method, although the user then types a code again on each other device.

## Consequences

- An unrevoked remembered-device token skips the MFA login step until its fixed 30-day expiry. Sensitive actions still need re-authentication.
- A person who has the password or the provider account and a device with an unexpired token signs in without the second factor. That person still cannot make a sensitive change without a TOTP code or a recovery code.
- After a lost device, the user revokes all remembered devices and signs in again with a code on each other device.
- Each client stores 1 more secret, and the web frontend needs a safe place for it.
- The Identity MFA remembered-device specification defines the fields, the methods, the limits, and the tests.
