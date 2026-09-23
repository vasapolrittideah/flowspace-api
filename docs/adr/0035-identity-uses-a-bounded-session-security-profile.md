# ADR-0035: Identity uses a bounded session security profile

## Status

Accepted

## Date

2026-09-24

## Context

[ADR-0031](0031-flowspace-owns-authentication-and-revocable-sessions.md) requires signed access tokens, public verification keys, revocable sessions, and authenticated internal checks. The [session spec](../specs/identity-password-login-and-sessions.md) leaves concrete security values open. The first environments have disposable data and one Identity replica. Automatic key rotation is part of the learning goal so that normal rollover and failure recovery can be exercised before real users join.

## Decision

Identity signs access tokens with Ed25519 (`EdDSA`). The issuer is exactly `flowspace-identity-local`, `flowspace-identity-staging`, or `flowspace-identity-production` in the matching environment. The audience is exactly `flowspace-api`, and the token type is `at+jwt`. Verifiers accept only Ed25519 keys from Identity, match the issuer, audience, type, and known `kid`, reject token-supplied key locations, and reject tokens whose `exp - iat` exceeds ten minutes. A future `iat` has at most 30 seconds of tolerance. An access token expires at `exp` with no positive tolerance, and Identity never extends session expiry for clock skew. These checks follow [RFC 8725](https://www.rfc-editor.org/info/rfc8725/).

The one Identity replica keeps an encrypted signing keyring on an Identity-only persistent volume, outside PostgreSQL and Git. AES-256-GCM protects the keyring with a separate key supplied through a workload-scoped Kubernetes Secret under [ADR-0027](0027-git-contains-only-encrypted-kubernetes-secrets.md). Keyring changes use atomic file replacement. Identity uses a `Recreate` rollout so two pods do not rotate the keyring together. On the first install, Identity creates a keyring only if the session store is empty. A missing or unreadable keyring otherwise stops token issuance. This storage remains in the single-host failure domain.

Identity serves public keys at `GET /.well-known/jwks.json` with `Cache-Control: public, max-age=60`. This standards-based document endpoint is a scoped exception to [ADR-0005](0005-one-protobuf-contract-generates-rest.md); the account and session APIs remain generated from Protobuf. JWKS contains only Ed25519 public keys with distinct key IDs under [RFC 8037](https://www.rfc-editor.org/info/rfc8037/). Verifiers fetch it from a fixed, configured internal HTTPS URL with a pinned trust root, never from a token header. They refresh on an unknown key ID at most once per five seconds per verifier, and reject new protected requests when their cached keys expire and Identity cannot refresh them. A successful live session check is never cached.

Identity rotates signing keys automatically every 24 hours. It persists and publishes a new public key at least two minutes before signing with it, and keeps the old public key for at least 11 minutes after its last signature. It removes the old private key after switching signers. Rotation resumes from persisted keyring state after a restart. If rotation fails, Identity keeps the current signer and reports the failure; it does not remove a key that may still be needed.

For suspected signing-key theft, the operator stops token issuance and revokes every session before removing the suspect public key from JWKS. Identity publishes a fresh key and waits at least two minutes for JWKS caches to expire before issuing new sessions. If revocation cannot commit, issuance remains stopped and protected services fail closed. A compromised host must be isolated and rebuilt before credentials are trusted again. The single-host key-theft exercise and independent recovery remain gates before real-user data.

Each protected service receives its own random 256-bit internal bearer credential. Identity stores only a verifier for each credential and permits it to call `CheckSession` only. The call uses internal TLS with a pinned trust root and a network policy that allows named service callers. The credential is distinct from user access tokens and never appears in logs. JWKS retrieval needs no bearer credential because it serves only public keys.

Password login uses two independent sliding-window limits: 30 attempts per source IP in 10 minutes and 10 attempts per normalized email identifier in 15 minutes. Every attempt counts, including requests for an unknown email. Identity stores the limits in its PostgreSQL database and stores a keyed digest rather than a plaintext email limit key. A limit returns a generic `ResourceExhausted` result, and a limit-store failure returns `Unavailable`. By default, Identity trusts no forwarding header and uses the peer IP. A deployment that uses a proxy must name trusted proxy peers and make the edge replace client-supplied forwarding headers before exposing the route; until then, limits apply to the ingress peer.

The signup module owns the initial account, challenge, and session schemas and the first token issuer. Password login and later methods extend that same session model after signup merges. Identity does not create a second session schema for login.

## Alternatives Considered

### Rotate keys by hand

- Pros: fewer keyring states and a simpler rollout.
- Cons: the first learning environment would not exercise automatic rollover or recovery.
- Rejected: routine automatic rotation is an explicit learning goal.

### Add a key or rate-limit service now

- Pros: it could support multiple Identity replicas later.
- Cons: it adds an operating dependency before the first one-replica experiment needs it.
- Rejected: the Identity volume and PostgreSQL meet the current scope.

### Trust every forwarding header

- Pros: less ingress configuration.
- Cons: a direct client could forge its source IP and bypass source limits.
- Rejected: only named proxy peers may provide a forwarded source address.

## Consequences

- A routine rollout briefly stops Identity because the first deployment uses one replica and `Recreate`.
- Loss of the single-host keyring stops issuance until recovery; it is not a real-user backup design.
- An attacker can exhaust one email limit and delay that account's login for up to 15 minutes. Review this denial-of-service trade-off before real users join.
- Internal TLS certificates, service credentials, and trusted proxy peers must be configured before the related routes are exposed.
