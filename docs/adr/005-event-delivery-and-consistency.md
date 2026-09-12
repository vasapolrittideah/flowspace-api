# ADR-005: Event delivery and consistency

## Status

Accepted.

## Date

2026-09-10.

## Context

Notifications and replicated authorization are the first asynchronous capabilities. Their availability and latency should not collapse the accepted service boundaries into synchronous runtime dependencies.

## Decision

Integrate services through asynchronous, versioned domain events. Workspace publishes membership and role changes for local authorization projections. Work publishes task events for Notifications. Use a broker with durable consumer offsets and registered Protobuf event schemas. Event contracts remain distinct from RPC contracts and use stable event identifiers.

When a domain mutation emits an event, commit the mutation and its outbox record in one transaction. Publish the outbox asynchronously with the same event ID on every retry. A successful task assignment means Work committed the assignment and its pending event; notification delivery may lag or be temporarily unavailable.

Consume events at least once. Notifications commits the processed event ID and inbox item in one transaction, enforced by event-ID uniqueness, then commits the broker offset. Broker guarantees alone do not create an atomic transaction with PostgreSQL or end-to-end exactly-once processing.

Every task content, status, and assignment mutation requires an expected task version. Reject stale versions as conflicts; clients reload or deliberately merge rather than silently overwriting another write. The selected broker, registry, and Go client are listed in [technology choices](../technology-choices.md).

## Alternatives Considered

Synchronous notification creation would couple task-write availability to Notifications. Change-data capture would add another infrastructure path and make event intent less explicit than an application outbox. Unversioned or unregistered payloads would defer the schema-evolution exercise. Broker transactions cannot include the consumer's database transaction. Last-write-wins would silently discard concurrent task edits.

## Consequences

Outbox backlog, consumer lag, duplicate delivery, poison records, replay, and stale-write conflicts are normal operating states that require metrics and tests. Retention for events, outbox rows, deduplication records, and schemas must cover the supported replay window. Broker, schema-registry, and database recovery form one tested recovery story.
