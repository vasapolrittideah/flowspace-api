# FlowSpace backend codebase structure

Status: accepted direction with implementation details still open.

Updated: 2026-09-12.

This document defines where backend code belongs and which dependency directions are allowed. It applies the service boundaries from the [architecture overview](architecture-overview.md), the repository layout from [ADR-002](adr/002-repository-and-go-module-layout.md), and the tools in [technology choices](technology-choices.md).

## 1. Principles

- Keep all application services in one repository and one root Go module.
- Organize service code around the Workspace, Work, and Notifications ownership boundaries.
- Keep business rules independent of transports, databases, brokers, identity providers, and deployment tooling.
- Keep each service's schema, queries, migrations, generated query code, and container build with that service.
- Put only cross-process interfaces in `contracts/`; keep service implementation types private.
- Put a technical package in root `internal/` only after multiple services genuinely need it.
- Keep tests beside the code they verify unless they exercise a deployed system or a cross-service workload.
- Never edit generated code by hand.
- Create directories only when their first real file is added; the trees below are targets, not empty scaffolding.

## 2. Target tree

```text
.
├── .github/
│   └── workflows/
├── contracts/
│   ├── events/
│   ├── http/
│   └── proto/
│       └── flowspace/<service>/v1/
├── deploy/
│   ├── base/
│   └── overlays/
│       ├── local/
│       ├── staging/
│       └── production/
├── docs/
│   └── adr/
├── gen/
│   └── go/
│       └── flowspace/<service>/v1/
├── internal/
├── scripts/
├── services/
│   ├── workspace/
│   ├── work/
│   └── notifications/
├── tests/
│   ├── load/
│   └── smoke/
├── buf.gen.yaml
├── buf.yaml
├── go.mod
├── go.sum
├── sqlc.yaml
├── Taskfile.yaml
└── Tiltfile
```

Tool configuration stays where the owning tool expects it. Optional output such as `contracts/http/`, event schemas, service directories, deployment overlays, and test suites appears only when the corresponding capability is implemented.

## 3. Repository directory ownership

| Path | Owner and contents |
| --- | --- |
| `services/<service>/` | One independently deployable application service, including its binaries, private packages, database assets, tests, and container build. |
| `contracts/proto/` | Source Protobuf definitions for synchronous service APIs, versioned by compatibility boundary. |
| `contracts/events/` | Source Protobuf schemas for asynchronous events, versioned separately from RPC contracts. |
| `contracts/http/` | Generated public HTTP contract output when a client or documentation workflow needs it. |
| `gen/go/` | Go code generated from shared contracts. |
| `internal/` | Technical packages with multiple concrete service consumers; never shared domain models or repositories. |
| `tests/smoke/` | Critical behavior checks against deployed service boundaries. |
| `tests/load/` | Controlled cross-service load experiments with explicit thresholds. |
| `deploy/base/` | Shared Kubernetes application resources. |
| `deploy/overlays/` | Local, staging, and production-specific Kustomize changes. |
| `scripts/` | Small repository automation used by local tasks or CI. |
| `docs/` | Current architecture and engineering guidance. |
| `docs/adr/` | Accepted architectural decisions and their rationale. |
| `.github/workflows/` | Repository CI workflows. |

Root `internal/` is not a default home for helpers. Code with one service owner stays under that service, even when another service might need something similar later.

## 4. Structure inside a service

Each service follows the hexagonal architecture established in [ADR-002](adr/002-repository-and-go-module-layout.md): business rules stay at the center and infrastructure stays at the edges.

```text
services/<service>/
├── cmd/
│   ├── api/
│   │   └── main.go
│   └── migrate/
│       └── main.go
├── db/
│   ├── migrations/
│   └── queries/
├── internal/
│   ├── adapter/
│   │   ├── in/
│   │   │   ├── event/
│   │   │   └── http/
│   │   └── out/
│   │       ├── event/
│   │       ├── keycloak/
│   │       └── postgres/
│   │           └── sqlc/
│   ├── app/
│   ├── bootstrap/
│   ├── domain/
│   └── port/
│       ├── in/
│       └── out/
└── Dockerfile
```

Not every service needs every directory:

- `cmd/api/` starts the service process and delegates construction to `bootstrap/`.
- `cmd/migrate/` runs that service's versioned migrations without becoming a second owner of migration logic.
- `domain/` defines business concepts, invariants, and domain errors without transport or infrastructure dependencies.
- `port/in/` defines the use cases exposed by the service and their transport-independent inputs.
- `app/` implements inbound ports, coordinates domain behavior, controls transaction boundaries, and calls outbound ports.
- `port/out/` defines capabilities required from databases, event brokers, identity providers, or other external systems.
- `adapter/in/` validates and translates an incoming protocol into calls to inbound ports.
- `adapter/out/` implements outbound ports with concrete infrastructure.
- `bootstrap/` loads typed configuration and wires concrete adapters to application code with ordinary constructors.
- `db/migrations/` contains versioned schema changes owned by the service.
- `db/queries/` contains handwritten SQL used by sqlc.
- `adapter/out/postgres/sqlc/` contains generated database access code.

Start each layer flat. Add a named integration directory only when the integration has a real implementation; a service that consumes no events does not need `adapter/in/event/`.

## 5. Dependency direction

Production dependencies point inward:

```text
cmd
 └── bootstrap
      ├── adapter/in ──> port/in, port/out, domain
      ├── app ─────────> port/in, port/out, domain
      └── adapter/out ─> port/out, domain

adapter/in/http ───────> gen/go
adapter/out/postgres ──> adapter/out/postgres/sqlc
```

Rules:

- Domain code imports no application, port, adapter, generated, or infrastructure package.
- Ports may use domain types but do not depend on adapters.
- Application code depends on ports and domain code, not concrete infrastructure.
- Inbound adapters depend on inbound ports and may use an outbound port needed to admit a request, such as token verification; outbound adapters implement outbound ports.
- Bootstrap is the composition root and may import concrete adapters and application constructors.
- Entry points parse process concerns and hand control to bootstrap; they do not contain business workflows.
- One service never imports another service's `internal/` packages.
- Cross-service synchronous calls use generated contract clients; asynchronous integration uses generated event contracts.
- Root shared packages never import a service package.

Generated types may appear at a transport boundary, but domain and application behavior must not depend on generated request or response messages.

## 6. Domain, application, and ports

- Put entity invariants and domain errors in `domain/` when they remain meaningful without a transport or database.
- Put use-case inputs and interfaces in `port/in/`; keep HTTP headers, Protobuf messages, SQL rows, and broker records out of them.
- Put orchestration in `app/`, including authorization decisions, calls to repositories, and coordination of domain changes.
- Add an outbound port only for a capability application code actually needs. Do not create an interface merely to mirror a concrete type.
- Keep service-owned domain models private. Cross-service identifiers are values at a contract boundary, not shared Go entities.

Transaction boundaries belong to the application use case. A database adapter may provide the mechanism, but it must not silently decide a business transaction.

## 7. Adapters and bootstrap

- HTTP and RPC handlers authenticate, enforce transport limits and deadlines, validate external input, translate generated messages, and map errors to canonical statuses.
- Event consumers validate envelopes, preserve event ordering and idempotency requirements, invoke an inbound use case, and commit offsets only according to the accepted delivery contract.
- PostgreSQL adapters implement repositories with handwritten queries and generated sqlc code.
- Identity, broker, and internal RPC clients stay in integration-specific outbound adapter directories.
- Bootstrap owns configuration parsing, client and pool construction, constructor wiring, server lifecycle, and graceful shutdown.

Adapters contain protocol and infrastructure behavior, not product rules. Bootstrap connects implementations; it does not become a service locator or a second application layer.

## 8. Contracts and generated code

- `contracts/proto/flowspace/<service>/v1/` is the source of truth for synchronous APIs in the `flowspace.<service>.v1` Protobuf package.
- Public methods carry `google.api.http` annotations; internal-only methods remain unannotated.
- `contracts/events/` owns versioned event schemas independently of RPC contracts.
- `gen/go/` is produced by Buf from source contracts and is replaced by regeneration, never manual edits.
- `contracts/http/` contains generated OpenAPI output only when a consumer or documentation workflow needs it; no handwritten OpenAPI contract duplicates Protobuf.
- Generated adapters and messages contain no business rules.

Contract changes keep source and generated output in the same change and must preserve the compatibility rules in [ADR-003](adr/003-api-and-contract-architecture.md). Transport-specific translation stays in an adapter instead of leaking into domain types.

## 9. Persistence ownership

- Each service owns its PostgreSQL instance, credentials, migrations, connection pool, storage, and recovery lifecycle.
- Schema changes live in `services/<service>/db/migrations/`; SQL queries live in the sibling `db/queries/` directory.
- sqlc output lives under the owning PostgreSQL adapter because it is an infrastructure implementation detail.
- Database constraints enforce local invariants; application code controls business transaction boundaries.
- A service never imports another service's repository or reads another service's database.
- Cross-service identifiers do not receive database foreign keys.

Keep migrations, queries, generated query code, and affected adapter behavior consistent. Migration execution remains a controlled step separate from code generation.

## 10. Tests

- Colocate Go tests as `*_test.go` beside the package they verify.
- Test domain invariants and application use cases without transport or infrastructure when possible.
- Test adapters against their real boundary where behavior depends on PostgreSQL, broker, identity, or protocol semantics; use Testcontainers for service-level dependencies when needed.
- Keep `tests/smoke/` for critical behavior across deployed services and `tests/load/` for controlled k6 experiments.
- Test generated behavior through source contracts and adapters; do not hand-maintain tests for mechanical generated code.
- Keep test helpers with their only consumer. Promote one to a narrowly named shared package only after multiple tests need the same behavior.

## 11. Deployment and repository tooling

- Keep each service's `Dockerfile` with the service so its build context and binary ownership remain visible.
- Put shared application manifests in `deploy/base/` and environment-specific changes in `deploy/overlays/<environment>/`.
- Keep environment credentials out of source; Git contains only encrypted secret manifests.
- Use root tool configuration for repository-wide generation and checks because the repository has one Go module.
- Put a script in `scripts/` only when a short command in `Taskfile.yaml` or the owning tool's configuration is insufficient.

Third-party infrastructure uses maintained, pinned packages as accepted in [ADR-008](adr/008-ci-cd-and-deployment.md). Its exact deployment layout is added when that infrastructure is configured rather than reserved in advance.

## 12. Placement checklist

Place a new backend file by asking these questions in order:

1. Is it a business invariant or domain error owned by one service? Put it in that service's `internal/domain/`.
2. Does it define or implement one service use case? Put the interface in `port/in/` and the implementation in `app/`.
3. Does application code need an external capability? Put its interface in `port/out/` and its implementation in `adapter/out/<integration>/`.
4. Does it receive HTTP, RPC, or event input? Put it in `adapter/in/<protocol>/`.
5. Is it service-owned SQL or schema? Put it in that service's `db/queries/` or `db/migrations/`.
6. Is it a cross-process source contract? Put it in `contracts/proto/` or `contracts/events/`.
7. Is it generated from a contract or query? Put it in the configured generator output and change the source instead.
8. Is it a technical package with multiple concrete service consumers? Put it in a narrowly named root `internal/<boundary>/` package.
9. Does it verify a deployed workflow or cross-service load? Put it in `tests/smoke/` or `tests/load/`.
10. Otherwise, keep it beside its only owner until another concrete need establishes a boundary.

## 13. Prohibited coupling and generic layers

Do not create these as default shared layers:

```text
pkg/
internal/common/
internal/helpers/
internal/models/
internal/repositories/
internal/utils/
```

Helpers, models, and repositories are valid, but ownership comes from a service or a narrowly named technical boundary rather than a generic suffix.

Also avoid:

- importing another service's private packages;
- sharing domain models or persistence repositories across services;
- direct access to another service's database;
- business rules in handlers, generated code, SQLC packages, entry points, or bootstrap;
- handwritten request or response models that duplicate Protobuf contracts;
- interfaces with no application-owned boundary or only a speculative implementation;
- per-service `go.mod` files or a root `go.work` file; and
- empty standard layer directories created only to match the target tree.

## 14. Growth rule

This structure grows by evidence:

- Add a directory when its first file has a clear owner.
- Add a port or adapter when a real inbound or outbound integration requires it.
- Promote technical code to root `internal/` only after multiple services demonstrate the shared contract.
- Add a service only when independent ownership and deployment justify a new boundary.
- Split the root Go module only when independent dependency management becomes a concrete need.
- Change service boundaries, top-level ownership, or module layout through an architecture decision rather than an incidental implementation change.
