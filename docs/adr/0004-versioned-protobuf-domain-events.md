# ADR-0004: Publish domain events as versioned Protobuf in a CloudEvents envelope

- **Status:** Accepted
- **Date:** 2026-09-05
- **Deciders:** Vasapol Rittideah
- **Tags:** `domain`, `data`

## Context

Services in FlowSpace communicate asynchronously as well as synchronously. The
asynchronous contract is the harder of the two to get right, for one reason
that does not apply to HTTP: **an event that has been written to the log cannot
be rewritten.** A consumer can be redeployed; the six months of history it is
about to replay cannot.

This makes the event schema the most expensive contract in the system. A
missing field is permanent, a reused field number silently corrupts old
messages, and a payload containing personal data makes deletion requests
unanswerable for as long as retention lasts.

Three decisions already taken depend on this one, so it cannot be deferred: the
outbox writes these messages inside the aggregate's transaction, the saga
orchestrator drives workflows off them, and the read models are rebuilt by
replaying them.

The broker itself is a separate decision. This ADR assumes only that it is a
partitioned, replayable log with ordering guaranteed per partition key and
retention long enough to rebuild a read model from scratch.

## Decision

### Encoding and envelope

Payloads are **Protobuf**, one message per event type, defined under
`proto/flowspace/<context>/v1/events.proto` and compiled with Buf. `buf lint`
and `buf breaking` run in CI against the main branch and block the merge.

Messages are wrapped in a **CloudEvents** envelope in binary content mode: the
attributes travel as broker headers, the Protobuf bytes are the body. This
keeps the metadata readable without deserialising the payload, which is what
the inbox needs in order to deduplicate.

```text
ce_id           018f4e6b-1d02-7a44-b91c-0f5e8d7a6b22
ce_type         dev.vasapol.flowspace.workitem.status_changed.v1
ce_source       /flowspace/workitem
ce_subject      wi_018f4e6b-1d02-7a44-b91c-0f5e8d7a6b22
ce_time         2026-09-05T10:14:02.113Z
ce_workspaceid  018f4e6a-9c1b-7c3d-8e2f-1a2b3c4d5e6f
traceparent     00-4bf92f3577b34da6-00f067aa0ba902b7-01
```

`ce_id` is the outbox row identifier, so producer retries and broker
redeliveries carry the same value and the inbox can reject the duplicate.
`traceparent` is mandatory: without it a trace ends at the producer and the
consumer's work looks like it came from nowhere.

### Naming and versioning

The type is `dev.vasapol.flowspace.<context>.<aggregate>.<past_tense>.v<major>`.
The major version lives in the type string and nowhere else.

- Additive change — a new optional field — does not bump the version.
- Breaking change publishes a **new type** suffixed `.v2` alongside `v1`. The
  producer emits both for a deprecation window; `v1` is never mutated and never
  removed while a consumer group still reads it.
- Field numbers are never reused; removals leave `reserved`. Every enum has an
  `_UNSPECIFIED = 0` member. These are enforced by `buf breaking`, not by
  review.

### What an event carries

An event carries its identifiers and **the field values that are part of the
fact itself** — the old and new status on a status change, the actor who caused
it. It carries nothing else. A consumer needing more reads the source of truth.

```protobuf
message WorkItemStatusChanged {
  string work_item_id = 1;
  string project_id   = 2;
  Status old_status   = 3;   // part of the fact: unreadable after the change
  Status new_status   = 4;
  string actor_id     = 5;
}
```

The test is whether reading the value back later would give a different answer
than the value at the moment of the event. If yes, it belongs in the payload,
because a read-back would be wrong. If no, leave it out and let the consumer
fetch it.

**Events carry no personal data.** No names, no email addresses, no free text
authored by a person. Identifiers only. A consumer that needs an email address
reads it from the identity service at the moment it sends. This is what keeps a
deletion request answerable while a replayable log exists.

### Topics, keys and tolerance

- One topic per aggregate: `flowspace.workitem`. The version is in the type,
  not the topic, so `v1` and `v2` stay in order relative to each other.
- The partition key is `workspace_id`, so the broker preserves the order in
  which the producer published, per tenant. That is a broker guarantee, not an
  end-to-end one: what the producer is able to publish in order is settled by
  the outbox decision, which guarantees ordering per aggregate only. Consumers
  must not depend on cross-aggregate ordering within a workspace. The cost of
  this key is that a dominant tenant becomes a hot partition, accepted for the
  same reason as in ADR-0002.
- A consumer that meets an unrecognised type **skips it and increments a
  counter**. It does not fail and does not dead-letter. The alternative is that
  publishing any new event type breaks every consumer already in production.

### The contract lives in git

There is no schema registry service. `proto/` plus the generated code in a
single repository is the registry, and `buf breaking` is the enforcement.

## Consequences

### Positive

- A breaking change fails CI rather than a consumer at 3am.
- The envelope makes deduplication, tracing and routing possible without
  parsing the body.
- A new read model can be built by replaying from the beginning of the topic.
- Deletion requests stay answerable, because the log holds no personal data.

### Negative / accepted costs

- **We cannot prove who still consumes `v1`.** Without a registry there is no
  runtime record of consumers, so retiring a version relies on consumer-group
  lag and a per-type counter, read by a human. This is the weakest point of
  this ADR and the first thing a registry would fix.
- Double-publishing during a version migration is real work in the producer and
  needs a deprecation window someone actually tracks.
- Thin events push reads back to the producing service; a consumer catching up
  after downtime generates a read burst against the system it is behind.
- Protobuf on the wire means a broker UI shows bytes. Debugging needs tooling.

### Neutral

- Topic and partitioning choices are broker-agnostic, so selecting the broker
  remains a separate and reversible decision.

## Alternatives considered

| Option                      | Why not                                                                                                                                                            |
| --------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| JSON with JSON Schema       | A removed or retyped field becomes a runtime failure in a consumer that is not deployed yet, and there is no build step that can prove otherwise                   |
| Avro with a schema registry | The right answer at scale, but it adds a stateful service to a 32 GB node and a second serialisation toolchain beside the one already required for synchronous RPC |
| Bare Protobuf, no envelope  | Every consumer reinvents id, time, trace context and the deduplication key, and the outbox and inbox need one uniform place to find them                           |
| Version as a payload field  | The consumer must successfully parse the message before it can decide whether it is able to parse the message                                                      |
| One topic for all events    | No per-aggregate retention or partition tuning, and every consumer pays to read every event in the system                                                          |

## Revisit when

- A consumer exists outside this repository, or a version retirement is
  blocked because nobody can say who still reads it. Both point at a schema
  registry.
- One workspace dominates a partition. Changing the key changes the ordering
  guarantee, so it ships as a new major version of the affected types, not as
  a configuration change.
- Replaying a topic takes longer than the outage the replay is meant to repair.
  That is the signal for compaction or periodic snapshots, not for a bigger
  broker.

## References

- [CloudEvents specification](https://cloudevents.io/)
- [CloudEvents Kafka protocol binding](https://github.com/cloudevents/spec/blob/main/cloudevents/bindings/kafka-protocol-binding.md)
- [Buf — breaking change detection](https://buf.build/docs/breaking/overview)
- [W3C Trace Context](https://www.w3.org/TR/trace-context/)
