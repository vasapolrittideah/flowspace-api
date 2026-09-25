# ADR-0036: Authenticate internal session checks with mutual TLS

## Status

Proposed

## Date

2026-09-25

## Context

[ADR-0031](0031-flowspace-owns-authentication-and-revocable-sessions.md) requires each protected service to ask Identity for live session state on every protected request. The [session specification](../specs/identity-password-login-and-sessions.md) requires an authenticated caller for `CheckSession` but leaves the method open. A public client must not call this method with a subject and session ID of its choice.

Identity now serves public gRPC on port 8080. Its separate internal listener serves health checks and public signing keys on port 8081 without TLS. Kubernetes NetworkPolicy can restrict connections, but it does not authenticate callers or encrypt traffic. The first release has no service mesh or certificate service.

## Decision

Use mutual TLS for `CheckSession`. Identity serves this RPC on a separate internal gRPC listener with TLS 1.3. The public REST gateway has no route for it, and the public gRPC listener rejects it. Expose the internal listener through a cluster-only Service and allow connections only from named protected-service pods through NetworkPolicy. Do not use the health and signing-key listener for `CheckSession`.

Identity requires and validates a client certificate against an environment-specific certificate authority (CA). It accepts only a certificate with the client-authentication use and one exact URI subject alternative name from its caller allowlist. The first allowed identity is `urn:flowspace:service:workspace`. The allowlist binds each identity to the SHA-256 fingerprint of its certificate's `SubjectPublicKeyInfo` bytes. Each later protected service needs its own certificate and allowlist entry. Identity takes the caller identity from the verified certificate, never from request metadata.

Each caller validates Identity's server certificate against the same environment's CA and the configured DNS name of the internal Service. The server certificate needs the server-authentication use and that DNS name. Neither side disables certificate or hostname validation. A TLS failure, an unknown caller, or an unavailable Identity service denies Workspace admission. Workspace reports a temporary service failure when it cannot complete the session check.

Keep the CA private key outside Git and the cluster. Mount each workload's private key from its own environment-specific Sealed Secret as a read-only file. Distribute the CA certificate and approved public-key fingerprints to verifiers. Issue replacement certificates before expiry. During planned rotation, allow both public-key fingerprints for a short overlap. After a client key leak, remove the old fingerprint from Identity's allowlist and restart Identity and the affected caller to close existing connections. A new certificate must have a new key. Do not log certificate private keys, access tokens, or full session-check requests.

Workspace still validates the user's access token locally before it sends the token's subject and session ID to `CheckSession`. Identity checks those values against its own session and account records. Caller authentication does not replace token validation, the live session check, or Workspace's membership rules.

## Alternatives Considered

### Use NetworkPolicy alone

- Pros: No certificates or extra listener.
- Cons: NetworkPolicy filters network traffic but does not prove which service made a request or protect the request in transit.
- Rejected: `CheckSession` accepts subject and session IDs, so a reachable caller must prove its identity.

### Use a bearer secret for each service

- Pros: Each service can use a distinct secret with a simple metadata check.
- Cons: TLS is still needed to protect the secret in transit, and secret rotation adds another process beside TLS certificate rotation.
- Rejected: Client certificates authenticate each service on the TLS connection without another credential.

### Add a service mesh

- Pros: A mesh can issue workload identities and encrypt service traffic.
- Cons: It adds a control plane and changes deployment and recovery for one internal RPC.
- Rejected: Go's TLS support covers the current need without a new runtime service.

## Consequences

- Identity needs a separate TLS listener and a cluster-only Service. The public listener must reject `CheckSession` even when a client calls its gRPC path directly.
- Each environment needs a CA, server and client certificates, a caller allowlist, and a rotation procedure. Certificate expiry or a failed rotation denies protected requests until repaired.
- A leaked Identity server key requires a new environment CA and new certificates because callers trust that CA. Restart the callers to close existing connections.
- Tests must cover missing, expired, untrusted, and wrong-service certificates; the public gRPC path; Identity outages; and the verified-email state returned after email verification.

## Sources

- [Go TLS client-certificate verification](https://pkg.go.dev/crypto/tls#ClientAuthType)
- [gRPC authentication guide](https://grpc.io/docs/guides/auth/)
- [Kubernetes NetworkPolicy limits](https://kubernetes.io/docs/concepts/services-networking/network-policies/)
