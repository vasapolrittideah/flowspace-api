# ADR-0006: Event contracts are versioned separately from RPC contracts

## Status

Accepted

## Date

2026-09-14

## Context

An RPC describes a request and its immediate response, while an event describes a committed fact that may be stored and replayed after the originating API has evolved. Reusing one message for both would couple two compatibility lifecycles and encourage consumers to depend on command-specific fields.

## Decision

Keep Protobuf event schemas separate from synchronous RPC contracts. Version event schemas on their own compatibility boundary, register them, and give every event a stable identifier. An event names a committed domain fact rather than reusing an RPC request or response message.

## Alternatives Considered

### Reuse RPC messages as events

- Pros: fewer schemas and mappings.
- Cons: a request-shaped message does not necessarily represent a committed fact, and RPC evolution can break replay consumers.
- Rejected: events and synchronous calls have different meanings and retention lifecycles.

### Publish unregistered payloads

- Pros: producers can add fields without schema tooling.
- Cons: compatibility failures appear only when a consumer reads the payload.
- Rejected: explicit schemas make evolution reviewable before publication.

## Consequences

- Producers maintain a mapping from domain facts to event contracts.
- RPC and event versions can evolve independently.
- Replay remains possible after synchronous API changes.
- Schema retention must cover the supported event replay window.
