# ADR-003: API and contract architecture

## Status

Accepted.

## Date

2026-09-09 (consolidated).

## Context

External clients need conventional REST/JSON APIs, while Go services need typed synchronous contracts. Maintaining separate public and internal request models would duplicate generation and compatibility work.

## Decision

Use Protobuf service definitions as the source of truth for synchronous APIs. Annotate methods that require public access with `google.api.http` and generate REST/JSON reverse proxies. Internal clients call the same service contracts over gRPC; generated adapters contain no business rules.

Keep event contracts separate from synchronous RPC contracts. Generate OpenAPI from annotated Protobuf only when a client or documentation workflow needs it; do not maintain a second handwritten API contract.

The selected generators and RPC implementation are listed in [technology choices](../technology-choices.md).

## Alternatives Considered

REST everywhere would lose the selected typed RPC exercise. RPC everywhere would not provide the chosen resource-oriented external interface. Separate OpenAPI and Protobuf sources would allow independent public models but recreate the duplication this decision removes.

## Consequences

Exposed contract changes must be reviewed for both REST and RPC compatibility. Internal-only methods remain unannotated. Authentication context, trace context, deadlines, idempotency, validation, authorization, and stable error mapping must cross the proxy boundary explicitly.
