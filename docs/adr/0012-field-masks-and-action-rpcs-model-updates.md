# ADR-0012: Field masks and action RPCs model updates

## Status

Accepted

## Date

2026-09-14

## Context

Ordinary field edits and domain transitions have different semantics. A generic update that sends a complete resource cannot distinguish omitted values from intentional clearing, while hiding transitions such as ownership transfer inside fields obscures their authorization and invariants.

## Decision

Use `PATCH` with a Protobuf `FieldMask` for partial resource updates. Model domain transitions such as ownership transfer as explicit RPCs with annotated resource-oriented HTTP actions rather than writable ordinary fields.

## Alternatives Considered

### Replace the complete resource with PUT

- Pros: one straightforward update shape.
- Cons: clients must send fields they did not intend to change and can overwrite concurrent values.
- Rejected: partial updates need an explicit statement of changed fields.

### Represent every transition as a field update

- Pros: fewer RPC methods.
- Cons: authorization, preconditions, and side effects are hidden inside a generic patch.
- Rejected: domain actions deserve contracts that name their intent.

## Consequences

- Clients supply an update mask for partial changes.
- Services reject unsupported or immutable paths explicitly.
- Domain actions can define their own authorization and preconditions.
- Contract review distinguishes data edits from state transitions.
