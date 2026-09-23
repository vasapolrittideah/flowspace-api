# ADR-0036: Identity delivers email through a Redpanda outbox

## Status

Accepted

## Date

2026-09-24

## Context

The [signup spec](../specs/identity-signup-and-email-verification.md) requires signup to return a session after a durable email request commits, even when Mailpit cannot accept mail. The first release uses disposable data. The project also wants to exercise broker outage, backlog, replay, and recovery during email delivery. [ADR-0017](0017-outbox-and-idempotent-consumers-deliver-events.md) establishes a transactional outbox for published events.

## Decision

Identity commits the account, first session, verification challenge, encrypted email delivery record, and outbox record in one PostgreSQL transaction. Signup returns its tokens after that commit and does not wait for Redpanda or SMTP. Code requests for verification and account claim use the same delivery path.

An Identity publisher sends a versioned, Schema Registry-backed Protobuf delivery command containing only an opaque `delivery_id` to a dedicated Redpanda topic. It marks the outbox record published only after Redpanda acknowledges it. The topic contains no email address, code, or message body. Identity has topic and consumer-group ACLs limited to this delivery flow. A broker outage leaves committed requests in the PostgreSQL outbox for later publication.

An Identity consumer reads the delivery ID, loads the record from Identity PostgreSQL, and checks that the challenge is still current and unexpired. It decrypts the code with a workload-scoped key kept outside PostgreSQL and separate from signing and code-verifier keys, then sends the message to Mailpit over SMTP. The challenge stores only a keyed one-way verifier; the encrypted delivery material is removed after a recorded send or when it expires or is replaced. A delivery that waits beyond the ten-minute code lifetime is discarded. The user can request a new code under the approved limits.

The consumer records a sent, expired, or replaced outcome before manually committing the Redpanda offset. It leaves the offset uncommitted while SMTP fails and retries with a bounded delay. A repeated broker command for a completed delivery is a no-op. SMTP and PostgreSQL cannot commit atomically: if Mailpit accepts a message and the consumer stops before recording that result, the same email may be sent again. Only one challenge per account and purpose remains valid, and a late email with a replaced code cannot verify an account. This is at-least-once delivery, not an exactly-once email promise.

## Alternatives Considered

### Send mail in the signup request

- Pros: no publisher or consumer is needed.
- Cons: Mailpit failure or delay would change the signup result after the account commit.
- Rejected: signup must return after the durable database commit.

### Send directly from a PostgreSQL worker

- Pros: fewer moving parts and no broker hop.
- Cons: it does not exercise the selected Redpanda outage and replay path.
- Rejected: learning broker recovery is part of this first experiment.

## Consequences

- Broker or Mailpit outages do not roll back signup. Delivery may lag until the code expires, after which a new code request is needed.
- Duplicate SMTP messages are possible after an ambiguous send result. Tests must prove that duplicate commands and stale emails cannot make two codes valid.
- Queue age, publish failures, consumer lag, SMTP failures, expired deliveries, and secret removal need tests and bounded telemetry.
