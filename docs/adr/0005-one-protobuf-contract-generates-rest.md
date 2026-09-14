# ADR-0005: One Protobuf contract generates the REST surface

## Status

Accepted

## Date

2026-09-14

## Context

External clients need conventional REST and JSON while Go services need typed synchronous contracts. Two handwritten surfaces would duplicate request models, mapping, validation, compatibility work, and documentation whenever a field changes.

## Decision

Use Protobuf service definitions as the source of truth for synchronous APIs. Public methods carry `google.api.http` annotations that generate REST and JSON reverse proxies; internal clients call the same service contracts over gRPC. Generated adapters contain no business rules. Use the standard Protobuf JSON mapping, and generate OpenAPI from the annotated Protobuf only when a client or documentation workflow needs it.

## Alternatives Considered

### Handwritten REST beside gRPC

- Pros: each surface can use independently optimized messages and routes.
- Cons: request models and domain mappings are duplicated and can drift.
- Rejected: the maintenance cost does not serve a current client requirement.

### RPC for every caller

- Pros: one generated transport and no HTTP translation.
- Cons: browser and conventional HTTP clients lose the selected resource-oriented interface.
- Rejected: the public API must remain accessible as REST and JSON.

## Consequences

- One contract change is reviewed for both REST and RPC compatibility.
- Public routes remain RPC-shaped and use lower-camel-case Protobuf JSON fields.
- Internal-only methods remain unannotated.
- Generated OpenAPI and gateway code are replaced by regeneration, never edited by hand.
