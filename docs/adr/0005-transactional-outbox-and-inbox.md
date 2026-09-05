# ADR-0005: Publish through a transactional outbox and consume through an inbox

- **Status:** Accepted
- **Date:** 2026-09-05
- **Deciders:** Vasapol Rittideah
- **Tags:** `domain`, `data`

## Context

A service that changes its own state and tells the rest of the system about it
performs two writes, to two systems, with no shared transaction. There is no
safe order for those two writes:

- Publish first, then commit: a crash in between leaves every other service
  acting on a fact that never happened. Nothing can detect this, because the
  service that would know has no record of it.
- Commit first, then publish: a crash in between loses the event. Local state
  is correct and internally consistent, so no reconciliation job will ever
  notice the gap.

This is the dual-write problem, and it is not solvable by ordering, retrying or
being careful. It is solved by making the intent to publish part of the same
transaction as the state change, which means it has to be a row in the same
database.

The delivery side has the mirror problem. The broker guarantees at-least-once,
and duplicates are not an exception path: a producer that retries after an
acknowledgement timeout, a consumer group rebalance, and any deliberate replay
all deliver a message a consumer has already seen. Exactly-once *delivery* does
not exist. Exactly-once *effect* does, if the consumer is idempotent.

ADR-0004 already supplies what is needed for both halves: `ce_id` is stable
across producer retries and broker redeliveries, so it is a deduplication key.

## Decision

### Producer: an outbox table written in the aggregate's transaction

```sql
create table outbox (
  id           uuid primary key,        -- UUIDv7, becomes ce_id
  workspace_id uuid not null,           -- partition key only
  topic        text not null,
  type         text not null,
  subject      text not null,
  payload      bytea not null,
  headers      jsonb not null,
  created_at   timestamptz not null default now(),
  published_at timestamptz
);

create index outbox_unpublished on outbox (id) where published_at is null;
```

The aggregate change and its outbox rows are one `COMMIT`. Either both are
durable or neither is, and no code path may write one without the other.

**The outbox is deliberately outside the tenant isolation model of ADR-0002.**
It carries `workspace_id` because the broker needs a partition key, but it has
no RLS policy and no composite foreign key, because the relay must read every
tenant's rows. The compensating control is the relay's database role: it can
read and update `outbox` and nothing else. It has no `SELECT` on any domain
table, so the exemption cannot be widened by accident.

### Relay: one leader, polling, in identifier order

A single leader-elected worker per service polls:

```sql
select id, topic, type, subject, payload, headers
from outbox
where published_at is null
order by id
limit 256
for update skip locked;
```

It publishes the batch, marks the rows published, and repeats every 200 ms.
Rows are deleted seven days after publication, so the partial index stays small
enough to remain in cache.

Two consequences of this design are contracts, not implementation details:

- **Ordering is guaranteed per aggregate, not per workspace.** Concurrent
  transactions touching different aggregates may commit in a different order
  than their identifiers were generated, so their events may reach the broker
  inverted. Two changes to the *same* aggregate cannot invert, because they
  serialise on the aggregate row. Consumers must not depend on cross-aggregate
  ordering within a workspace.
- **The outbox blocks head-of-line, on purpose.** A message the broker keeps
  rejecting stops that service's publishing until a human intervenes. Skipping
  it would silently discard a fact that the database says happened, which is
  the exact failure this ADR exists to prevent.

`LISTEN`/`NOTIFY` would cut publish latency from a poll interval to nearly
nothing, and is not used: it is session-scoped and therefore incompatible with
the transaction-mode pooling required by ADR-0002. Buying it means giving the
relay a direct connection that bypasses the pooler, which is a trade to make
when latency is measured, not assumed.

The health of this whole mechanism is one number:
`outbox_oldest_unpublished_seconds`. It is the service level indicator for
publication, and it catches a dead leader, a wedged broker and a poison message
alike.

### Consumer: an inbox keyed by consumer and event

```sql
create table inbox (
  consumer     text not null,
  event_id     uuid not null,           -- ce_id from ADR-0004
  processed_at timestamptz not null default now(),
  primary key (consumer, event_id)
);
```

Every handler runs one transaction:

```text
begin
  insert into inbox (consumer, event_id) values ($1, $2)
    on conflict do nothing        -- zero rows means duplicate: stop here
  apply the effect
commit
commit the broker offset          -- only after the transaction commits
```

Committing the offset last keeps the failure mode on the safe side: a crash
before the offset commit redelivers a message the inbox will then reject.

**This gives exactly-once effect only for effects that are writes to the same
database.** An effect that leaves the database — sending an email, calling a
third-party API — is at-least-once no matter what this table does, because the
external call and the local commit are themselves a dual write. Such calls must
carry their own idempotency key and the remote side must honour it. Pretending
otherwise is where most "exactly-once" systems actually lose.

The inbox defends against redelivery, not against intentional replay. Rebuilding
a read model from the start of a topic truncates that consumer's inbox rows and
its projection together, as one operation.

## Consequences

### Positive

- No lost events and no phantom events, from a mechanism that is one table and
  one loop rather than a distributed transaction.
- Duplicate delivery becomes a non-event for database effects.
- Publication health is a single observable number with a clear alert.
- The relay reads the same table a log-decoding replacement would read, so
  moving to change data capture later does not change the event contract.

### Negative / accepted costs

- Every service gains a table, a background worker and a leader election.
- Publish latency has a floor of the poll interval, and the fix is blocked by
  the pooling decision in ADR-0002 until someone measures that it matters.
- A poison message stops publication for its service by design. This is a page,
  not a retry, and it must be understood before it happens at 3am.
- The relay role is a documented hole in the tenant isolation model, kept
  narrow by privilege rather than by the schema.
- The inbox grows with consumed throughput and needs its own retention job.

### Neutral

- Ordering is weaker than ADR-0004's partition key alone suggests. This ADR is
  where the real guarantee is defined.

## Alternatives considered

| Option                                   | Why not                                                                                                                                                                        |
| ---------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| Publish, then write the database         | A crash in between leaves the rest of the system believing a fact that never happened, and nothing in the system can detect it                                                 |
| Write the database, then publish         | A crash in between loses the event silently; the local state is correct, so no reconciliation will ever notice                                                                 |
| Two-phase commit                         | Requires a transaction coordinator with its own durable log, blocks every participant while the coordinator is down, and no broker under consideration speaks XA to PostgreSQL |
| Change data capture on the domain tables | Couples the public event contract to the physical table layout, so a column rename becomes a breaking change for consumers                                                     |
| Idempotent consumers with no outbox      | Solves duplicate delivery, which is a different problem from lost publication; the two are orthogonal and both are needed                                                      |

## Revisit when

- The p99 of `outbox_oldest_unpublished_seconds` exceeds what an in-product
  feature needs. The next step is logical decoding over the same outbox table,
  which changes the transport and not the contract.
- One relay cannot keep up with its service. Sharding by a hash of
  `workspace_id` across several relays is available, at the cost of the
  ordering guarantee above, so it ships as a new major event version.
- Inbox insertion measurably contends with domain writes on the same database.

## References

- [Transactional outbox pattern](https://microservices.io/patterns/data/transactional-outbox.html)
- [Idempotent consumer pattern](https://microservices.io/patterns/communication-style/idempotent-consumer.html)
- [PostgreSQL SELECT — FOR UPDATE SKIP LOCKED](https://www.postgresql.org/docs/current/sql-select.html#SQL-FOR-UPDATE-SHARE)
