# ADR-0011: Idempotency keys protect non-idempotent creates

## Status

Accepted

## Date

2026-09-14

## Context

A client can lose the response to a successful create and cannot tell whether retrying will duplicate the effect. Network retries are normal, so callers need a durable way to distinguish a replay from a different request using the same key.

## Decision

Require an `Idempotency-Key` header for create operations whose effects are not naturally idempotent, beginning with workspace creation, and forward it explicitly through the REST proxy. Scope the key to the authenticated subject and method, claim it atomically, bind it to a request hash, replay a completed result, and reject a changed payload or an in-flight duplicate as a conflict. Retain records for at least the operation's documented maximum retry window.

## Alternatives Considered

### Let clients retry creates without a key

- Pros: no storage or protocol addition.
- Cons: an ambiguous response can produce duplicate durable effects.
- Rejected: the server is the only component able to determine whether the original operation committed.

### Deduplicate by request payload alone

- Pros: callers do not manage keys.
- Cons: two intentional identical creates become indistinguishable while semantically equivalent payloads can hash differently.
- Rejected: caller-selected operation identity is explicit and stable.

## Consequences

- Protected creates require durable idempotency records and atomic uniqueness.
- Replays receive the original completed result.
- A reused key with a different payload fails rather than changing meaning.
- Storage retention is part of each protected operation's retry contract.
