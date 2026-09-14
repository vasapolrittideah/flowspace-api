# ADR-0009: Canonical gRPC errors map to HTTP

## Status

Accepted

## Date

2026-09-14

## Context

The same application failure can reach a sibling over RPC and a public client over REST. Separate error contracts would let equivalent failures drift across transports and could expose internal details through one surface.

## Decision

Return canonical gRPC status codes with standard structured error details and use the gateway's default HTTP status mapping. Validate external input at the transport boundary and identify invalid fields without exposing internal details. Do not define a separate custom HTTP error envelope.

## Alternatives Considered

### A handwritten HTTP error envelope

- Pros: the public API can use a shape designed independently of gRPC.
- Cons: it creates a second error contract and a second mapping to maintain.
- Rejected: the generated REST surface should preserve the canonical RPC classification.

### Independent REST and RPC error mappings

- Pros: each transport can choose its own error semantics.
- Cons: the same failure can produce different statuses depending on how it was called.
- Rejected: the generated REST surface should preserve the RPC status semantics.

## Consequences

- RPC and REST callers observe consistent failure categories.
- Validation errors can name fields without revealing service internals.
- Unclassified failures become internal errors and require diagnostic logs or traces.
- Clients depend on standard status semantics instead of a FlowSpace-specific envelope.
