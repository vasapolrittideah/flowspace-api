# Technology choices

These tables are the source of truth for replaceable implementation tools and Go packages. These selections support the accepted architecture but do not each warrant an ADR. Replace one when measured limitations or maintenance cost justify it, while preserving the owning ADR's contract.

## Platforms and tools

| Area | Selection | Role / replacement boundary | ADR |
| --- | --- | --- | --- |
| Application language | Go | Application services; changing language must preserve service contracts and ownership boundaries. | [ADR-001](adr/001-service-architecture.md), [ADR-002](adr/002-repository-and-go-module-layout.md) |
| Contract tooling | Buf CLI | Generate, lint, and check compatibility for Protobuf contracts. | [ADR-003](adr/003-api-and-contract-architecture.md) |
| Relational database | PostgreSQL | Primary transactional application data. | [ADR-004](adr/004-persistence-architecture.md) |
| Query generation | sqlc | Generate typed Go methods from handwritten SQL. | [ADR-004](adr/004-persistence-architecture.md) |
| Event broker | Redpanda | Durable asynchronous events and consumer offsets. | [ADR-005](adr/005-event-delivery-and-consistency.md) |
| Event schemas | Protobuf + Redpanda Schema Registry | Version and register event payloads separately from RPC contracts. | [ADR-005](adr/005-event-delivery-and-consistency.md) |
| Identity provider | Keycloak | OIDC identity, authentication, recovery, and federation flows. | [ADR-006](adr/006-identity-and-authorization.md) |
| Test email | Mailpit | Capture learning-environment verification and recovery email. | [ADR-006](adr/006-identity-and-authorization.md) |
| Server Kubernetes | K3s | Kubernetes distribution on Ubuntu. | [ADR-007](adr/007-runtime-environments.md) |
| Local Kubernetes | k3d | Kubernetes cluster inside local Docker Desktop. | [ADR-007](adr/007-runtime-environments.md) |
| Local container runtime | Docker Desktop | Run the local k3d cluster on Mac. | [ADR-007](adr/007-runtime-environments.md) |
| Local orchestration | Tilt | Build, deploy, inspect, and port-forward the local k3d loop. | [ADR-007](adr/007-runtime-environments.md) |
| Initial volumes | K3s Local Path Provisioner | Node-local learning-environment persistence. | [ADR-007](adr/007-runtime-environments.md) |
| Cluster ingress | Traefik (K3s bundled) | Route tunnel traffic to Kubernetes services; exact origin path remains proposed. | [ADR-009](adr/009-edge-access-and-secrets.md) |
| Source and CI | GitHub + GitHub Actions | Public source/reviews and hosted CI within the zero-spend limit. | [ADR-008](adr/008-ci-cd-and-deployment.md) |
| Image registry | GHCR | Public versioned FlowSpace service images. | [ADR-008](adr/008-ci-cd-and-deployment.md) |
| GitOps reconciler | Argo CD | Pull desired Kubernetes state from Git. | [ADR-008](adr/008-ci-cd-and-deployment.md) |
| Application manifests | Kustomize | Shared base with local, staging, and production overlays. | [ADR-008](adr/008-ci-cd-and-deployment.md) |
| Infrastructure packaging | Helm | Reuse maintained, pinned third-party charts. | [ADR-008](adr/008-ci-cd-and-deployment.md) |
| API smoke tests | Postman + Postman CLI | Author and automate public API smoke collections. | [ADR-008](adr/008-ci-cd-and-deployment.md) |
| Load tests | k6 | Controlled load experiments with explicit thresholds. | [ADR-008](adr/008-ci-cd-and-deployment.md) |
| Go static analysis | golangci-lint | Repository-wide formatting and selected analyzers. | [ADR-008](adr/008-ci-cd-and-deployment.md) |
| Dependency updates | Renovate | Propose reviewed dependency-update pull requests; no automerge. | [ADR-008](adr/008-ci-cd-and-deployment.md) |
| Public edge | Cloudflare Tunnel + Access | Outbound application ingress and the staging admission gate. | [ADR-009](adr/009-edge-access-and-secrets.md) |
| Kubernetes secrets | Sealed Secrets | Store encrypted secret manifests for in-cluster decryption. | [ADR-009](adr/009-edge-access-and-secrets.md) |
| Telemetry collection | Grafana Alloy | Collect and forward application and platform telemetry. | [ADR-010](adr/010-observability.md) |
| Metrics | Prometheus | Self-hosted metrics storage and queries. | [ADR-010](adr/010-observability.md) |
| Logs | Loki | Self-hosted centralized logs. | [ADR-010](adr/010-observability.md) |
| Traces | Tempo | Self-hosted distributed traces. | [ADR-010](adr/010-observability.md) |
| Dashboards | Grafana | Query and visualize metrics, logs, and traces. | [ADR-010](adr/010-observability.md) |

## Go packages

| Area | Package | Role / replacement boundary | ADR |
| --- | --- | --- | --- |
| Environment configuration | `github.com/caarlos0/env/v11` | Parse typed service settings from environment variables; replacement must preserve validation, defaults, and secret handling. | [ADR-002](adr/002-repository-and-go-module-layout.md), [ADR-007](adr/007-runtime-environments.md) |
| Structured logging | `go.uber.org/zap` | Emit structured JSON application logs; replacement must preserve stable fields and lifecycle flushing. | [ADR-010](adr/010-observability.md) |
| Internal RPC | `connectrpc.com/connect` | Generated synchronous service clients and handlers over gRPC. | [ADR-003](adr/003-api-and-contract-architecture.md) |
| Public API proxy | `github.com/grpc-ecosystem/grpc-gateway/v2` | Generate annotated REST/JSON routes from Protobuf contracts. | [ADR-003](adr/003-api-and-contract-architecture.md) |
| PostgreSQL driver | `github.com/jackc/pgx/v5` | Runtime database access from Go. | [ADR-004](adr/004-persistence-architecture.md) |
| SQL migrations | `github.com/pressly/goose/v3` | Run service-owned versioned SQL migrations. | [ADR-004](adr/004-persistence-architecture.md) |
| Event client | `github.com/twmb/franz-go` | Publish and consume Redpanda events. | [ADR-005](adr/005-event-delivery-and-consistency.md) |
| Integration tests | `github.com/testcontainers/testcontainers-go` | Provide disposable real dependencies for service-level tests. | [ADR-008](adr/008-ci-cd-and-deployment.md) |
| Instrumentation | `go.opentelemetry.io/otel` | Provide vendor-neutral telemetry and context propagation. | [ADR-010](adr/010-observability.md) |
