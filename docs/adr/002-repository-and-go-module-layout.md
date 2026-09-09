# ADR-002: Repository and Go module layout

## Status

Accepted.

## Date

2026-09-09 (consolidated).

## Context

One developer and AI agents maintain related services, contracts, deployment definitions, and documentation. The services still need independent binaries and deployments.

## Decision

Keep FlowSpace in one repository with one `go.mod` at the root. Each application service has its own binary, container image, database assets, and service-specific `internal` packages. Do not add per-service modules or a `go.work` file initially.

Root `internal/` is reserved for technical helpers that are genuinely reused across services. Do not share domain models or persistence repositories between services, and create directories only when code needs them.

### Intended layout

```text
flowspace-api/
├── go.mod
├── go.sum
├── internal/                # Shared technical packages, added only when needed
├── services/
│   ├── workspace/
│   │   ├── cmd/api/main.go
│   │   ├── internal/
│   │   │   ├── domain/            # Aggregates, invariants, and domain errors
│   │   │   ├── port/{in,out}/     # Use-case and repository or gateway contracts
│   │   │   ├── app/               # Use cases implementing port/in
│   │   │   ├── adapter/
│   │   │   │   ├── in/http/         # REST/JSON surface
│   │   │   │   ├── out/keycloak/
│   │   │   │   └── out/postgres/    # Repository; sqlc/ holds generated queries
│   │   │   └── bootstrap/         # Typed configuration and manual wiring
│   │   ├── db/{migrations,queries}/
│   │   └── Dockerfile
│   ├── work/                  # Same layout
│   └── notifications/         # Same layout
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

## Alternatives Considered

Separate repositories or Go modules allow independent dependency versions and governance but add coordination that does not serve this one-maintainer project yet.

## Consequences

Services share a Go dependency graph while retaining separate builds and releases. Repository-wide checks can run from the root. Split modules only when independent dependency management becomes a concrete need.
