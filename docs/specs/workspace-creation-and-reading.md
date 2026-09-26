# Spec: Workspace creation and reading

Module id: `workspace-creation-and-reading`

Status: Approved.

## Objective

Build the first Workspace capability for signed-in users of FlowSpace. A user can create a workspace and read a workspace where that user is a member. Safe retries must prevent duplicate creation after a lost response.

This spec assumes that workspace creation and reading do not exist yet. It defines required behavior and completion evidence. It does not report implementation progress or establish deployment readiness.

Create and read belong to one capability because they share workspace data, ownership, and access rules. The first implementation uses Keycloak for authenticated identity. Work and Notifications are not dependencies of this capability.

## Scope and decision sources

The scope covers `CreateWorkspace` and `GetWorkspace`, owner membership at creation, token admission, durable retry protection, and service-owned persistence.

The feature details below form the approved contract. Open proposals in the [architecture](../architecture.md) remain undecided.

This capability excludes workspace listing, updates, archival, invitations, membership management, role changes, and ownership transfer. It also excludes frontend work, new login flows, and Work or Notifications features.

Authorization event publication and downstream projections belong to later integration work under [ADR-0020](../adr/0020-services-authorize-from-bounded-local-projections.md). This capability authorizes reads from Workspace-owned membership data. It does not establish readiness for downstream authorization.

## Contract

Use package `flowspace.workspace.v1` and service `WorkspaceService`. Protobuf definitions are the source of truth for RPC and generated REST/JSON under [ADR-0005](../adr/0005-one-protobuf-contract-generates-rest.md). Version routes under `/v1` under [ADR-0007](../adr/0007-version-apis-by-compatibility-boundary.md).

| RPC | Public HTTP route | Request | Successful response |
| --- | --- | --- | --- |
| `CreateWorkspace` | `POST /v1/workspaces` | `CreateWorkspaceRequest` with required `name` | `CreateWorkspaceResponse` with `workspace`, HTTP 200 |
| `GetWorkspace` | `GET /v1/workspaces/{workspace_id}` | `GetWorkspaceRequest` with required `workspace_id` | `GetWorkspaceResponse` with `workspace`, HTTP 200 |

Both methods require exactly one `Authorization: Bearer <access_token>` header. Creation also requires exactly one `Idempotency-Key` header. The REST proxy must forward both headers explicitly.

A UUID is a unique resource identifier. The server uses it to identify each workspace. Clients use the returned ID for later reads.

If a public HTTP request body exceeds 1 MiB, return HTTP 413 before it reaches the use case. If its JSON is malformed, return HTTP 400 without creation effects.

The resource `Workspace` contains these fields:

| Protobuf field | JSON field | Meaning |
| --- | --- | --- |
| `id` | `id` | Server-generated UUID, output only |
| `name` | `name` | Workspace name after removal of surrounding whitespace |
| `created_at` | `createdAt` | Server-generated creation time, output only, using `google.protobuf.Timestamp` |

An illustrative create request and response use standard Protobuf JSON mapping:

```http
POST /v1/workspaces
Authorization: Bearer <access_token>
Idempotency-Key: <unique_request_key>
Content-Type: application/json

{"name":"Platform"}
```

```json
{"workspace":{"id":"550e8400-e29b-41d4-a716-446655440000","name":"Platform","createdAt":"2026-09-17T00:00:00Z"}}
```

The same resource wrapper applies to a successful read. Clients do not supply the acting subject, owner role, resource ID, or creation time in the create request.

## Required behavior

### Identity and access

A subject is a stable identity identifier. A membership links a subject to a workspace role. Store subjects as identity references instead of email addresses.

Apply these identity and access rules:

- Make sure that token signature, issuer, audience, expiry, and subject are valid before admitting either operation.
- Derive the acting subject from that token under [ADR-0013](../adr/0013-acting-identity-comes-from-the-token.md).
- The first implementation uses Keycloak under superseded [ADR-0019](../adr/0019-keycloak-owns-authentication-flows.md). Future authentication follows [ADR-0031](../adr/0031-flowspace-owns-authentication-and-revocable-sessions.md).
- The first implementation allows any authenticated subject to create a workspace. Under ADR-0031, Workspace must require a verified email when Identity replaces Keycloak. Creation makes that subject the workspace owner.
- Allow one subject to own or join multiple workspaces, with a separate role in each.
- Require a membership for the authenticated subject in the requested workspace before a read. Viewer, member, admin, and owner memberships all allow this read.
- Do not use client-supplied subjects, workspace IDs, or role claims as proof of access.

### Creation and persistence

Remove surrounding Unicode whitespace from `name`. Require valid UTF-8 and between 1 and 100 Unicode code points after that removal. Reject empty names, whitespace-only names, and names above the limit.

A Unicode code point identifies one character value. A visible character can contain more than one code point. Apply the name limit to code points rather than bytes.

The server generates the UUID and creation time. Identical names do not identify the same workspace. Separate requests with different idempotency keys can create separate workspaces with the same name.

Commit the workspace, its owner membership, and its completed retry record in one transaction. A successful creation must leave exactly one owner. A failure before commit must leave none of these records from that attempt.

Workspace owns its PostgreSQL instance, schema, migrations, and queries under [ADR-0014](../adr/0014-relational-data-uses-explicit-sql.md) and [ADR-0015](../adr/0015-each-service-owns-a-postgresql-instance.md). Local constraints must prevent duplicate memberships and multiple owners. Application code controls the transaction boundary without depending on a concrete database adapter.

### Retry protection

Idempotency means that retries preserve one operation's result. Follow [ADR-0011](../adr/0011-idempotency-keys-protect-non-idempotent-creates.md). Apply these retry rules:

- Require 1 to 255 visible ASCII characters (`0x21` through `0x7E`). Treat the accepted key as opaque and compare it byte-for-byte without case folding or normalization.
- Scope each key to the authenticated subject and `CreateWorkspace` method. Different subjects can use the same key independently.
- Bind the key to a hash of the normalized request, after name whitespace removal.
- Claim the key atomically so concurrent requests cannot commit duplicate effects.
- If the subject, key, and normalized name match a completed request, return its original resource and successful status. Preserve its ID, name, and creation time.
- If the normalized name changes for the same subject and key, return a conflict without creating another workspace.
- If an attempt still holds the key, reject an overlapping duplicate as a conflict.
- Release an in-progress claim when its transaction rolls back or its database connection closes, including after a process crash. After release, a retry can attempt creation again.
- If a response is lost after commit, return the committed result on retry.
- Set the logical expiry of a completed record to 24 hours after its successful commit. Persist the record and original result across process restarts until that expiry.
- At or after logical expiry, the same subject and key can identify a new operation. Physical cleanup can occur later, but an expired record must not replay or conflict.

### Reading and errors

Require a nonempty `workspace_id`. For a malformed UUID, a missing workspace, or a subject without membership, return the same `NotFound` result. Do not reveal whether an inaccessible workspace exists.

Use canonical gRPC errors and the gateway's default HTTP mapping under [ADR-0009](../adr/0009-canonical-grpc-errors-map-to-http.md). Invalid fields use standard `google.rpc.BadRequest` details. Responses must not expose SQL, credentials, tokens, or internal diagnostics.

| Condition | gRPC status | HTTP status |
| --- | --- | --- |
| Missing or invalid authentication | `Unauthenticated` | 401 |
| Invalid request field or missing, blank, duplicate, or oversized idempotency key | `InvalidArgument` | 400 |
| Malformed REST JSON | Not applicable | 400 |
| REST request body above 1 MiB | Not applicable | 413 |
| Malformed workspace UUID, missing workspace, or no membership | `NotFound` | 404 |
| Key reused with a different normalized request | `AlreadyExists` | 409 |
| Duplicate creation still in progress | `Aborted` | 409 |
| Effective deadline expires | `DeadlineExceeded` | 504 |
| Unexpected internal failure | `Internal` | 500 |

Honor shorter caller deadlines and cap ordinary requests at five seconds under [ADR-0010](../adr/0010-cap-ordinary-unary-requests-at-five-seconds.md). Propagate cancellation and the effective deadline to token verification and database work. A timeout does not prove that a create failed to commit, so clients retry with the same key.

### Request diagnostics

Emit structured request logs with service, environment, request ID, operation, outcome, status, and duration. Follow the architecture's `X-Request-ID` rule for accepted characters and the limit of 1 to 128 characters. Replace missing or invalid values, forward the effective ID, and return it in the response header.

Propagate trace context through the REST proxy and synchronous calls under [ADR-0028](../adr/0028-telemetry-is-vendor-neutral-and-correlated.md). Keep telemetry bounded and prevent export failures from blocking requests under [ADR-0030](../adr/0030-telemetry-is-bounded-and-non-blocking.md). Collector deployment and storage configuration remain outside this capability.

## Commands

Run commands from the repository root. Go tools need the repository's pinned toolchain. Integration tests need a running Docker runtime, and generation can need network access.

| Purpose | Command |
| --- | --- |
| Compile Workspace binaries without writing output binaries | `go build ./services/workspace/cmd/...` |
| Run Workspace tests | `go test ./services/workspace/...` |
| Run real PostgreSQL integration tests | `go test -tags=integration ./services/workspace/internal/adapter/out/postgres` |
| Lint source contracts | `task buf -- lint` |
| Generate API code | `task buf -- generate` |
| Make sure that API compatibility holds against main | `task buf -- breaking --against '.git#branch=main'` |
| Generate query code | `task sqlc -- generate` |

These commands define future implementation checks. Approval does not mean that the capability passes them.

## Testing strategy

Use Testcontainers with PostgreSQL for behavior that depends on transactions or database constraints. Test the public REST mapping and typed gRPC behavior through generated contracts and adapters.

Cover these concerns at their owning test boundary:

- Domain tests cover whitespace removal, UTF-8, empty names, the 100-code-point boundary, and multibyte names.
- Use-case tests cover authenticated subjects, input rejection, exact idempotency-key validation, dependency errors, and cancellation.
- Token tests cover wrong signatures, issuers, audiences, expired tokens, and missing subjects.
- Transport tests cover malformed JSON, oversized bodies, required headers, field error details, status mapping, and header forwarding. Make sure that rejected input never reaches creation. Exercise request IDs, deadlines, and cancellation through the public gateway.
- Database tests prove atomic owner creation, rollback, durable replay, logical expiry, claim release after connection loss, conflicts, concurrent duplicates, and membership-based reads. Create memberships directly in test setup for each role. This test setup does not introduce membership-management endpoints.

## Boundaries

### Always

Follow these rules for every implementation change:

- Derive actors from validated tokens.
- Enforce membership on reads.
- Preserve atomic creation.
- Protect retries.
- Honor deadlines.

### Ask first

Obtain approval for these decisions:

- Obtain approval before changing the approved scope, contract, retry window, or accepted decisions.

### Never

Do not take these actions:

- Do not trust client actor or role claims.
- Do not access another service's database.

The approved scope includes the service-owned schema and migrations needed for creation, memberships, and replay records. Broader schema changes and deployment decisions outside this scope require review. Do not build unrelated capabilities to complete this one.

## Success criteria

Each Given cell states the setup and operation. Each Then cell states the required observable result. Completion requires every row:

| Given | Then |
| --- | --- |
| An authenticated subject sends a valid create through REST or RPC. | The response contains the normalized name, a server-generated UUID, and a creation timestamp. Exactly one owner membership persists for that subject. |
| An authenticated subject sends a create with an invalid name or idempotency key. | The operation returns `InvalidArgument` (HTTP 400) without creation effects. |
| A caller uses either method with missing or invalid authentication. | The operation returns `Unauthenticated` (HTTP 401) without creation effects. |
| A create commits and its owner immediately reads the workspace. | The owner receives the committed resource. |
| The service restarts with the same database after a committed create, and the owner reads the workspace. | The owner receives the same resource. |
| The same subject retries a completed create with the same key and normalized name within the 24-hour retry window. | The operation returns the original resource without duplicate workspace or membership records. |
| A completed creation record has not expired, and the same subject reuses its key with a different normalized name. | The operation returns `AlreadyExists` (HTTP 409) without creating another workspace. |
| A completed creation record reaches its 24-hour logical expiry, and the same subject reuses its key. | The key can identify a new operation even if physical cleanup has not removed the expired record. |
| A duplicate create overlaps an attempt that still holds the same subject's key. | The duplicate returns `Aborted` (HTTP 409), and the attempts cannot commit duplicate effects. |
| A process crashes while its create attempt holds an in-progress claim. | The claim is released with the transaction or connection, and a retry can attempt creation without partial effects. |
| A different subject uses the same key, or the same subject uses a new key, for a valid create. | The operation can create a separate workspace independently. |
| An injected failure occurs before the creation transaction commits. | The attempt leaves no partial workspace, owner membership, or completed retry record. |
| A REST create contains malformed JSON or a body above 1 MiB. | Malformed JSON returns HTTP 400. An oversized body returns HTTP 413. Neither request reaches creation. |
| A subject with a viewer, member, admin, or owner membership reads that workspace through REST or RPC. | The operation returns the workspace using the same access rules for both transports. |
| An authenticated subject requests a workspace without membership, a nonexistent workspace, or a malformed UUID. | The operation returns the same `NotFound` (HTTP 404) result without revealing whether an inaccessible workspace exists. |
| A client supplies identity or role claims to either method through REST or RPC. | Access decisions use the validated token subject and Workspace-owned membership data. Client claims do not prove access. |
| A request through the generated REST boundary has a shorter caller deadline or exceeds the five-second cap. | The service honors the effective deadline and propagates it to dependencies. Expiry returns `DeadlineExceeded` (HTTP 504). |
| A caller cancels a request through the generated REST boundary. | Cancellation propagates to token verification and database work. |
| A request crosses the generated REST boundary with a valid, missing, or invalid request ID. | The service preserves a valid ID or replaces a missing or invalid ID. It forwards and returns the effective ID. |
| A request fails through the generated REST boundary. | The response uses the specified status and safe error details without exposing internal diagnostics. |
| The capability is submitted for review. | Applicable builds, tests, coverage, lint, security, contract generation, and compatibility checks meet the contribution policy and constraints. |

These are requirements for future evidence. No criterion is marked complete in this approved specification.
