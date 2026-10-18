# ADR-0049: Opt-in TOTP and recovery codes protect Identity accounts

Date: 2026-10-18

Status: Accepted; scoped exception to [ADR-0011](0011-idempotency-keys-protect-non-idempotent-creates.md)

## Context

A stolen password or provider account gives full access to a Flowspace account today. [ADR-0031](0031-flowspace-owns-authentication-and-revocable-sessions.md) and the [Identity threat model](../security/identity-threat-model.md) require multi-factor authentication (MFA) before real users join, and the threat model asks for a new review of enrollment and recovery.

Password login and provider login issue tokens in 1 step, under [ADR-0033](0033-identity-token-issuance-does-not-replay-responses.md) and [ADR-0034](0034-provider-login-uses-a-one-time-handoff.md). The [Identity re-authentication](../specs/identity-re-authentication.md) specification gives each account 3 equal step-up methods: a password, a linked provider, and an email code. Each of them works without a second factor.

No record states which second factor Identity uses, when it asks for it, or how a user who loses the factor gets back in.

## Decision

Each Identity account can turn on MFA with a time-based one-time password (TOTP) and 10 recovery codes. MFA is optional for every account, and no role requires it.

The TOTP follows RFC 6238 with SHA-1, 6 digits, and a 30-second step. Identity accepts the current step and 1 step on each side, and it accepts each step at most once for each account. Identity generates a 160-bit secret and stores it encrypted with an Identity key, because it must read the secret to check a code. A user turns MFA on in 2 steps: Identity returns the secret, and the user confirms it with 1 valid code before MFA starts.

For an account with MFA, every password login and provider login that creates a session needs a second step. `CreatePasswordSession` and `CreateProviderSession` return a single-use MFA challenge instead of tokens. The challenge expires 5 minutes after Identity issues it. A new RPC takes the challenge with a TOTP code or a recovery code and creates the session. A refresh needs no second factor.

A recovery code replaces a TOTP code once, at login or at re-authentication. Each code has 10 random letters and digits. Identity shows the codes once and stores only a hash of each code. A new set replaces the whole old set, and MFA stays on when the user has used all the codes.

For an account with MFA, re-authentication accepts only a TOTP code or a recovery code. A login that passes the second factor opens the re-authentication window, as other logins do. Turning MFA off, making a new set of recovery codes, and turning MFA on are sensitive methods. Turning MFA off deletes the secret and the recovery codes, and it revokes no other session. To move to a new authenticator app, the user turns MFA off and then on again.

When the account email is verified, Identity sends a notice when MFA turns on, when MFA turns off, and each time someone uses a recovery code. A notice contains no secret and no code.

A password reset through an email code does not turn MFA off, so the next login still needs the second factor. A user who loses both the TOTP device and every recovery code cannot get back into the account. Identity has no support path and no delayed recovery.

The MFA login step, the confirmation of MFA, and a new set of recovery codes reject `Idempotency-Key`, because Identity cannot replay tokens or plaintext recovery codes. This is a scoped exception to ADR-0011, as ADR-0033 is for password login. After a lost response, the client logs in again or makes a new set of recovery codes.

## Alternatives Considered

### MFA only for password logins

- Pros: provider logins stay 1 step, and users who rely on Google or GitHub see no change.
- Cons: a person who takes over the provider account of a user still gets into Flowspace without the second factor.
- Rejected: MFA belongs to the Flowspace account, so it protects every way into that account.

### Email codes as the second factor

- Pros: Identity already sends email codes, so the user needs no authenticator app.
- Cons: the password-recovery code already goes to the same mailbox, so a person with the mailbox has both factors.
- Rejected: a TOTP device is independent of the mailbox, and it works offline without a mail delay.

### WebAuthn or passkeys

- Pros: they resist phishing, and the user types no code.
- Cons: they need browser APIs and a web flow that API clients cannot use today.
- Rejected: TOTP works for API clients now, and a later record can add WebAuthn beside it.

### Password reset turns MFA off

- Pros: a user who loses the TOTP device and the codes can get back in through the email.
- Cons: a person with the mailbox of the user can turn MFA off and take over the account.
- Rejected: the mailbox is already the recovery factor for the password, so it must not also remove the second factor.

## Consequences

- An account with MFA resists a stolen password or a stolen provider account alone.
- Login responses get a second shape, so each client must handle the MFA challenge before it gets tokens.
- Identity holds a new encryption key for TOTP secrets. If the key is lost, Identity cannot check TOTP codes, but MFA stays on. Users can still log in with recovery codes, and restoring the key restores the TOTP check.
- A user who loses the TOTP device and every recovery code loses the account. The account deletion grace period does not help, because its restricted session also needs the second factor.
- The specifications of password login, provider login, re-authentication, and account deletion change to add the second step. The MFA specifications define the limits, the delivery of the notices, and the threat-model review.
- A later record decides a remembered device that skips the second factor for a time.

## Sources

- [RFC 6238: TOTP: Time-Based One-Time Password Algorithm](https://www.rfc-editor.org/rfc/rfc6238)
- [OWASP Multifactor Authentication Cheat Sheet](https://cheatsheetseries.owasp.org/cheatsheets/Multifactor_Authentication_Cheat_Sheet.html)
