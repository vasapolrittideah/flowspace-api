# ADR-0017: An outbox and idempotent consumers deliver events

## Status

Accepted

## Date

2026-09-14

## Context

A domain mutation and broker publication cannot share one atomic transaction. A crash after committing only one side can either lose a fact or publish a fact whose state never committed. Retried delivery can also apply one event more than once unless the receiver includes deduplication in its own transaction.

## Decision

Commit a domain mutation and its outbox record in one database transaction. Publish the outbox asynchronously with the same event ID on every retry. Consume events at least once. A consumer commits the processed event ID and its local effect in one transaction, enforced by event-ID uniqueness, before committing the broker offset.

Do not claim end-to-end exactly-once delivery; broker transactions cannot include a consumer's PostgreSQL transaction.

## Alternatives Considered

### Write state and then publish directly

- Pros: no outbox table or relay.
- Cons: a crash between the two operations loses publication or announces uncommitted state if their order is reversed.
- Rejected: the source database transaction is the only atomic boundary available to the mutation.

### Depend on broker exactly-once guarantees

- Pros: less deduplication logic in consumers.
- Cons: the broker cannot atomically commit the consumer's database effect.
- Rejected: end-to-end idempotency must include the receiving transaction.

## Consequences

- Outbox backlog and duplicate delivery require metrics and tests.
- A retry preserves one stable event identity.
- Consumers store deduplication records for at least the supported replay window.
- A committed source mutation can wait in the outbox while the broker is unavailable.
