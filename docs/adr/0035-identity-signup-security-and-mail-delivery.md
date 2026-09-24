# ADR-0035: Identity signup security and mail delivery

## Status

Accepted

## Date

2026-09-25

## Context

[ADR-0031](0031-flowspace-owns-authentication-and-revocable-sessions.md) requires signed access tokens and live session checks. The [session specification](../specs/identity-password-login-and-sessions.md) fixes a ten-minute access-token lifetime, a 30-day idle session limit, and a 90-day absolute session limit, but leaves signing details open. The [signup specification](../specs/identity-signup-and-email-verification.md) fixes password and code limits but leaves the blocklist source, source-address trust, and queued-code protection open.

Signup must commit a code delivery request with the account. A retry may run after the worker sends mail but before it records success. The broker must not become another store for codes or email addresses.

## Decision

### Access tokens and signing keys

- Sign access JWTs with Ed25519 (`alg=EdDSA`) and publish only Ed25519 public keys in Identity's JWKS. Reject every other JWT algorithm, unknown `kid`, wrong token type, issuer, or audience. The first local release uses the exact issuer `urn:flowspace:identity:local` and audience `flowspace-api`. Each later environment needs its own exact issuer and signing key; protected services configure the expected issuer and audience, never take either from the token as authority.
- Allow at most 30 seconds of clock tolerance when checking JWT `iat` and `nbf`. Reject a token at or after its `exp` without tolerance. Identity uses server time and no tolerance for code expiry or the 30-day and 90-day session limits. A successful live session check remains mandatory for protected requests.
- Generate each Ed25519 key pair from a cryptographically secure source outside Git and CI. Assign a unique `kid`. Store the private key in an environment-specific Sealed Secret and mount it as a read-only file only in Identity processes that sign tokens. Do not put it in PostgreSQL, an environment variable, an image, a log, or a protected service. Publish the matching public key at `/.well-known/jwks.json` on Identity's internal listener with a cache lifetime of at most 60 seconds. Verifiers fetch it from a configured location, never a token-supplied URL; a failed refresh cannot admit a token with an unknown key.
- For the disposable-data first release, start with one active key per environment. For a planned replacement, publish the new public key before switching the signer, then retain the previous public key until ten minutes, the approved 30-second clock tolerance, and the 60-second JWKS cache lifetime have passed since its last use. Keep both keys available through that overlap; record the `kid` and switch time without recording private material. If a private key may be compromised, stop signing with it, remove its public key, revoke affected sessions, and require login after the replacement is published. Test both paths before real users join. These procedures do not change the approved token or session lifetimes.

### Source address, limits, and passwords

- Store signup, code-request, and code-guess counters in Identity PostgreSQL with atomic updates so all replicas enforce the specification's limits. If the store is unavailable, fail the request instead of bypassing a limit.
- Read the source IP from the connection peer unless that peer belongs to an explicitly configured trusted proxy CIDR. For a trusted peer, use the rightmost untrusted address in a valid `X-Forwarded-For` chain and require a valid chain. The public edge must append the actual peer address and strip `Forwarded` and `X-Real-IP`; Identity rejects requests carrying either of those headers or a malformed `X-Forwarded-For` chain. Do not configure a wildcard trusted CIDR. Count direct requests and missing or ineligible accounts by the resulting source IP.
- Use Have I Been Pwned Pwned Passwords range API as the first-release compromised-password source. After NFC normalization, hash the complete candidate locally with SHA-1, send only its first five hexadecimal characters over HTTPS with response padding, and compare the returned suffixes locally. Also reject a small versioned list of common and FlowSpace-specific passwords. Never send or log the password or its full hash. If the range check fails or times out, reject password creation rather than accept an unchecked password. Apply the same check to signup, account claim, and later password changes. Keep the approved password length rules and Argon2id parameters.

### Queued email codes

- In the signup transaction, store the challenge's keyed one-way code verifier and a separate encrypted delivery payload in Identity PostgreSQL with the outbox record. Encrypt the code and recipient with AES-256-GCM using a fresh random nonce and authenticated challenge ID, purpose, and subject as associated data. Use a dedicated, versioned encryption key independent of the verifier and signing keys. Keep it in an environment-specific Sealed Secret mounted only in the Identity API and mail worker. The relay cannot read that key.
- Publish only a stable opaque event ID and challenge ID to the private Identity Redpanda topic. No code, ciphertext, password, token, or full email address belongs in the broker record, logs, or traces. The mail worker locks the account and current challenge in a database transaction, checks the challenge ID, purpose, account state, and ten-minute expiry, and decrypts the payload. It sends only if the decrypted recipient matches the current account address. Keep the lock through the bounded mail call and delivery-state commit so replacement and claim cannot overtake a send. Discard a replaced or expired event without sending.
- Delete encrypted delivery material after Mailpit accepts the current message, when the challenge is replaced or consumed, and at expiry. Run expiry cleanup at startup and at least once per minute while the worker is healthy; purge expired material after a backup restore before delivery starts. If mail delivery fails while the challenge is current, keep its encrypted payload for retry. If a crash occurs after Mailpit accepts the message but before the worker records success, retrying may send the same code again. The verifier and challenge remain single use; delivery does not create another valid code. Expired work is discarded, and the user may request a new code under the approved limits.

## Alternatives Considered

### Put the code in the broker event

- Pros: The worker can send mail without reading Identity storage.
- Cons: Broker retention and replay would retain a usable code outside Identity's key and deletion boundary.
- Rejected: Opaque identifiers keep delivery material in Identity storage.

### Keep only the code verifier

- Pros: Identity would not store a decryptable code.
- Cons: A delivery retry could not send the same code after a mail outage or worker crash.
- Rejected: Authenticated encryption protects the short-lived retry payload.

### Keep limits in process memory

- Pros: No database counter is needed.
- Cons: A restart or second replica would reset the effective limits.
- Rejected: Shared PostgreSQL state enforces the approved limits across replicas.

## Consequences

- The first release needs protected key bootstrap and an available Pwned Passwords check before it can accept passwords. This release still serves only disposable data. Real-user use needs the separate recovery and backup gates in the threat model.
- A ten-minute code can expire during an extended broker or mail outage. Signup remains committed; the user requests another code after recovery.
- The later password-session capability must reuse this token issuer, algorithm, JWKS route, key procedure, and shared session limits. Its internal `CheckSession` authentication method still needs approval before implementation.

## Sources

- [RFC 8037: EdDSA in JOSE](https://www.rfc-editor.org/rfc/rfc8037)
- [RFC 8725: JWT best current practices](https://www.rfc-editor.org/rfc/rfc8725)
- [NIST SP 800-63B: password blocklists](https://pages.nist.gov/800-63-4/sp800-63b.html#passwordver)
- [Pwned Passwords range API](https://haveibeenpwned.com/API/v3#SearchingPwnedPasswordsByRange)
- [OWASP cryptographic storage guidance](https://cheatsheetseries.owasp.org/cheatsheets/Cryptographic_Storage_Cheat_Sheet.html)
