# ADR-003: API and contract architecture

## Status

Accepted.

## Date

2026-09-09 (consolidated); API conventions accepted 2026-09-10.

## Context

External clients need conventional REST/JSON APIs, while Go services need typed synchronous contracts. Maintaining separate public and internal request models would duplicate generation and compatibility work.

## Decision

Use Protobuf service definitions as the source of truth for synchronous APIs. Annotate methods that require public access with `google.api.http` and generate REST/JSON reverse proxies. Internal clients call the same service contracts over gRPC; generated adapters contain no business rules.

Keep event contracts separate from synchronous RPC contracts. Generate OpenAPI from annotated Protobuf only when a client or documentation workflow needs it; do not maintain a second handwritten API contract.

The selected generators and RPC implementation are listed in [technology choices](../technology-choices.md).

### API conventions

- Version Protobuf packages by compatibility boundary, beginning with `flowspace.<service>.v1`, and expose public REST routes below `/v1` using plural resource nouns. Evolve a version additively; incompatible changes require a new package and route version.
- Use the standard Protobuf JSON mapping, including lower-camel-case field names.
- Paginate every list method with a bounded `page_size`, an opaque `page_token`, and a response `next_page_token`. Define a deterministic order and the default and maximum page sizes in each collection contract. Do not return total counts unless a product use case requires them.
- Return canonical gRPC status codes with standard structured error details and use gRPC-Gateway's default HTTP status mapping. Validation errors identify invalid fields; internal details are never exposed.
- Validate external input at the transport boundary before invoking application logic.
- Honor a caller deadline when it is shorter and cap ordinary unary requests at five seconds. Operations that cannot reliably finish within that bound require a separately designed contract rather than a larger implicit timeout.
- Require an `Idempotency-Key` header for create operations whose effects are not naturally idempotent, beginning with workspace creation, and forward it explicitly through the gateway. Scope the key to the authenticated subject and method, claim it atomically, bind it to a request hash, replay a completed result, and return a conflict for a changed payload or an in-flight duplicate. Retain keys for at least the operation's documented maximum retry window.
- Use `PATCH` with a Protobuf `FieldMask` for partial resource updates. Model domain transitions such as ownership transfer as explicit RPCs with annotated resource-oriented HTTP actions.
- Derive the acting identity from the validated token subject. Request bodies and paths never supply trusted actor or role claims.

## Alternatives Considered

REST everywhere would lose the selected typed RPC exercise. RPC everywhere would not provide the chosen resource-oriented external interface. Separate OpenAPI and Protobuf sources would allow independent public models but recreate the duplication this decision removes. Offset pagination and unconditional total counts would expose unstable positions and unnecessary query cost. A custom HTTP error envelope would create a second error contract beside canonical gRPC statuses. Unbounded request time and unsafe retries would leave callers unable to resolve slow or ambiguous outcomes.

## Consequences

Exposed contract changes must be reviewed for both REST and RPC compatibility. Internal-only methods remain unannotated. Authentication context, trace context, deadlines, idempotency, validation, authorization, and stable error mapping must cross the proxy boundary explicitly. List implementations must preserve token validity and deterministic ordering across supported requests. Idempotent create operations require durable records and an atomic uniqueness mechanism in the service-owned database.
