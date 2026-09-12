# Codebase structure

FlowSpace uses one repository and one Go module, as decided in [ADR-002](adr/002-repository-and-go-module-layout.md). Directories are added only when implementation needs them.

## Intended layout

```text
flowspace-api/
├── go.mod
├── go.sum
├── internal/                # Shared technical packages, added only when needed
├── services/
│   ├── workspace/
│   ├── work/
│   └── notifications/
├── contracts/
│   ├── http/
│   ├── proto/
│   └── events/
├── tests/
│   ├── smoke/
│   └── load/
├── deploy/
│   ├── base/
│   └── overlays/{local,staging,production}/
├── docs/
│   └── adr/
└── .github/workflows/
```

## Folder responsibilities

- `internal/` contains technical packages genuinely shared by multiple services. Domain models and repositories stay inside their owning service.
- `services/` contains the independently deployable Workspace, Work, and Notifications services. Each service uses the same internal structure.
- `contracts/` contains interfaces shared across process boundaries.
  - `http/` contains generated public HTTP contract output when needed.
  - `proto/` contains synchronous Protobuf service definitions.
  - `events/` contains asynchronous event schemas.
- `tests/smoke/` verifies critical behavior across deployed services; `tests/load/` contains controlled load experiments. Unit and service-level tests stay beside the code they verify.
- `deploy/base/` contains shared Kubernetes resources; `deploy/overlays/` contains local, staging, and production-specific changes.
- `docs/` contains current architecture and engineering guidance; `docs/adr/` records accepted architectural decisions and their rationale.
- `.github/workflows/` contains repository CI workflows.

## Hexagonal structure inside each service

Each service follows the hexagonal direction accepted in [ADR-001](adr/001-service-architecture-and-boundaries.md): business rules stay at the center and infrastructure stays at the edges.

```text
services/<service>/
├── cmd/
│   ├── api/main.go
│   └── migrate/main.go
├── internal/
│   ├── domain/
│   ├── port/
│   │   ├── in/
│   │   └── out/
│   ├── app/
│   ├── adapter/
│   │   ├── in/http/
│   │   └── out/<integration>/
│   └── bootstrap/
├── db/
│   ├── migrations/
│   └── queries/
└── Dockerfile
```

- `cmd/` contains executable entry points such as the API server and migration runner.
- `internal/` keeps service implementation packages private to that service.
- `domain/` defines business concepts, invariants, and domain errors without transport or storage dependencies.
- `port/in/` defines the use cases exposed by the service; `app/` implements them.
- `port/out/` defines capabilities the application needs from databases or external systems.
- `adapter/in/` translates incoming protocols into calls to inbound ports.
- `adapter/out/` implements outbound ports using PostgreSQL, Keycloak, or other infrastructure.
- `bootstrap/` loads typed configuration and wires concrete adapters to application code with ordinary constructors.
- `db/migrations/` contains versioned schema changes owned by the service.
- `db/queries/` contains handwritten SQL used to generate typed access code with sqlc.

Dependencies point inward: adapters depend on ports and application code, while domain code does not depend on adapters. Add a port or adapter only when a real integration requires it.
