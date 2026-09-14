# Technology stack

These tables are the source of truth for replaceable implementation tools and Go packages. These selections support the accepted architecture but do not each warrant an ADR. Replace one when measured limitations or maintenance cost justify it, while preserving the owning ADR's contract.

## Platforms and tools

| Area | Selection | Role / replacement boundary | ADR |
| --- | --- | --- | --- |
| Application language | Go | Application services; changing language must preserve service contracts and ownership boundaries. | [ADR-0001](adr/0001-one-bounded-context-per-service.md), [ADR-0002](adr/0002-one-repository-and-one-go-module.md) |
| Contract tooling | Buf CLI | Generate, lint, and check compatibility for Protobuf contracts. | [ADR-0005](adr/0005-one-protobuf-contract-generates-rest.md), [ADR-0007](adr/0007-version-apis-by-compatibility-boundary.md) |
| Relational database | PostgreSQL | Primary transactional application data. | [ADR-0014](adr/0014-relational-data-uses-explicit-sql.md), [ADR-0015](adr/0015-each-service-owns-a-postgresql-instance.md) |
| Query generation | sqlc | Generate typed Go methods from handwritten SQL. | [ADR-0014](adr/0014-relational-data-uses-explicit-sql.md) |
| Event broker | Redpanda | Durable asynchronous events and consumer offsets. | [ADR-0016](adr/0016-cross-service-side-effects-use-domain-events.md), [ADR-0017](adr/0017-outbox-and-idempotent-consumers-deliver-events.md) |
| Event schemas | Protobuf + Redpanda Schema Registry | Version and register event payloads separately from RPC contracts. | [ADR-0006](adr/0006-version-events-separately-from-rpc.md), [ADR-0016](adr/0016-cross-service-side-effects-use-domain-events.md) |
| Identity provider | Keycloak | OIDC identity, authentication, recovery, and federation flows. | [ADR-0019](adr/0019-keycloak-owns-authentication-flows.md) |
| Test email | Mailpit | Capture learning-environment verification and recovery email. | [ADR-0019](adr/0019-keycloak-owns-authentication-flows.md) |
| Server Kubernetes | K3s | Kubernetes distribution on Ubuntu. | [ADR-0021](adr/0021-run-kubernetes-locally-and-on-the-self-hosted-server.md) |
| Local Kubernetes | k3d | Kubernetes cluster inside local Docker Desktop. | [ADR-0021](adr/0021-run-kubernetes-locally-and-on-the-self-hosted-server.md) |
| Local container runtime | Docker Desktop | Run the local k3d cluster on Mac. | [ADR-0021](adr/0021-run-kubernetes-locally-and-on-the-self-hosted-server.md) |
| Local orchestration | Tilt | Build, deploy, inspect, and port-forward the local k3d loop. | [ADR-0021](adr/0021-run-kubernetes-locally-and-on-the-self-hosted-server.md) |
| Initial volumes | K3s Local Path Provisioner | Node-local learning-environment persistence. | [ADR-0022](adr/0022-single-host-storage-holds-disposable-data.md) |
| Cluster ingress | Traefik (K3s bundled) | Route tunnel traffic to Kubernetes services; exact origin path remains proposed. | [ADR-0026](adr/0026-an-outbound-tunnel-exposes-selected-routes.md) |
| Source and CI | GitHub + GitHub Actions | Public source/reviews and hosted CI within the zero-spend limit. | [ADR-0023](adr/0023-hosted-ci-stays-within-the-zero-spend-limit.md) |
| Image registry | GHCR | Public versioned FlowSpace service images. | [ADR-0024](adr/0024-gitops-deploys-versioned-images.md) |
| GitOps reconciler | Argo CD | Pull desired Kubernetes state from Git. | [ADR-0024](adr/0024-gitops-deploys-versioned-images.md) |
| Application manifests | Kustomize | Shared base with local, staging, and production overlays. | [ADR-0025](adr/0025-environments-overlay-shared-manifests.md) |
| Infrastructure packaging | Helm | Reuse maintained, pinned third-party charts. | [ADR-0025](adr/0025-environments-overlay-shared-manifests.md) |
| API smoke tests | Postman + Postman CLI | Author and automate public API smoke collections. | [ADR-0023](adr/0023-hosted-ci-stays-within-the-zero-spend-limit.md) |
| Load tests | k6 | Controlled load experiments with explicit thresholds. | [ADR-0023](adr/0023-hosted-ci-stays-within-the-zero-spend-limit.md) |
| Go static analysis | golangci-lint | Repository-wide formatting and selected analyzers. | [ADR-0023](adr/0023-hosted-ci-stays-within-the-zero-spend-limit.md) |
| Dependency updates | Renovate | Propose reviewed dependency-update pull requests; no automerge. | [ADR-0023](adr/0023-hosted-ci-stays-within-the-zero-spend-limit.md) |
| Public edge | Cloudflare Tunnel + Access | Outbound application ingress and the staging admission gate. | [ADR-0026](adr/0026-an-outbound-tunnel-exposes-selected-routes.md) |
| Kubernetes secrets | Sealed Secrets | Store encrypted secret manifests for in-cluster decryption. | [ADR-0027](adr/0027-git-contains-only-encrypted-kubernetes-secrets.md) |
| Telemetry collection | Grafana Alloy | Collect and forward application and platform telemetry. | [ADR-0028](adr/0028-telemetry-is-vendor-neutral-and-correlated.md), [ADR-0029](adr/0029-the-observability-stack-is-self-hosted.md) |
| Metrics | Prometheus | Self-hosted metrics storage and queries. | [ADR-0029](adr/0029-the-observability-stack-is-self-hosted.md) |
| Logs | Loki | Self-hosted centralized logs. | [ADR-0029](adr/0029-the-observability-stack-is-self-hosted.md) |
| Traces | Tempo | Self-hosted distributed traces. | [ADR-0029](adr/0029-the-observability-stack-is-self-hosted.md) |
| Dashboards | Grafana | Query and visualize metrics, logs, and traces. | [ADR-0029](adr/0029-the-observability-stack-is-self-hosted.md) |

## Go packages

| Area | Package | Role / replacement boundary | ADR |
| --- | --- | --- | --- |
| Environment configuration | `github.com/caarlos0/env/v11` | Parse typed service settings from environment variables; replacement must preserve validation, defaults, and secret handling. | [ADR-0003](adr/0003-hexagonal-layers-inside-each-service.md), [ADR-0021](adr/0021-run-kubernetes-locally-and-on-the-self-hosted-server.md) |
| Structured logging | `go.uber.org/zap` | Emit structured JSON application logs; replacement must preserve stable fields and lifecycle flushing. | [ADR-0028](adr/0028-telemetry-is-vendor-neutral-and-correlated.md) |
| Internal RPC | `connectrpc.com/connect` | Generated synchronous service clients and handlers over gRPC. | [ADR-0005](adr/0005-one-protobuf-contract-generates-rest.md) |
| Public API proxy | `github.com/grpc-ecosystem/grpc-gateway/v2` | Generate annotated REST/JSON routes from Protobuf contracts. | [ADR-0005](adr/0005-one-protobuf-contract-generates-rest.md) |
| PostgreSQL driver | `github.com/jackc/pgx/v5` | Runtime database access from Go. | [ADR-0014](adr/0014-relational-data-uses-explicit-sql.md) |
| SQL migrations | `github.com/pressly/goose/v3` | Run service-owned versioned SQL migrations. | [ADR-0014](adr/0014-relational-data-uses-explicit-sql.md) |
| Event client | `github.com/twmb/franz-go` | Publish and consume Redpanda events. | [ADR-0016](adr/0016-cross-service-side-effects-use-domain-events.md), [ADR-0017](adr/0017-outbox-and-idempotent-consumers-deliver-events.md) |
| Integration tests | `github.com/testcontainers/testcontainers-go` | Provide disposable real dependencies for service-level tests. | [ADR-0023](adr/0023-hosted-ci-stays-within-the-zero-spend-limit.md) |
| Instrumentation | `go.opentelemetry.io/otel` | Provide vendor-neutral telemetry and context propagation. | [ADR-0028](adr/0028-telemetry-is-vendor-neutral-and-correlated.md) |
