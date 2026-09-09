# Technology choices

This table is the source of truth for replaceable implementation tools. These selections support the accepted architecture but do not each warrant an ADR. Replace one when measured limitations or maintenance cost justify it, while preserving the owning ADR's contract.

| Area | Selection | Role / replacement boundary |
| --- | --- | --- |
| Application language | Go | Application services; changing language must preserve service contracts and ownership boundaries. |
| Internal RPC | ConnectRPC | Generated synchronous service clients and handlers over gRPC. |
| Public API proxy | gRPC-Gateway | Generate annotated REST/JSON routes from Protobuf contracts. |
| Contract tooling | Buf CLI | Generate, lint, and check compatibility for Protobuf contracts. |
| Relational database | PostgreSQL | Primary transactional application data. |
| Query generation | sqlc | Generate typed Go methods from handwritten SQL. |
| PostgreSQL driver | pgx | Runtime database access from Go. |
| SQL migrations | Goose | Run service-owned versioned SQL migrations. |
| Event broker | Redpanda | Durable asynchronous events and consumer offsets. |
| Event schemas | Protobuf + Redpanda Schema Registry | Version and register event payloads separately from RPC contracts. |
| Go event client | franz-go | Publish and consume Redpanda events. |
| Identity provider | Keycloak | OIDC identity, authentication, recovery, and federation flows. |
| Test email | Mailpit | Capture learning-environment verification and recovery email. |
| Server Kubernetes | K3s | Kubernetes distribution on Ubuntu. |
| Local Kubernetes | k3d | Kubernetes cluster inside local Docker Desktop. |
| Local container runtime | Docker Desktop | Run the local k3d cluster on Mac. |
| Local orchestration | Tilt | Build, deploy, inspect, and port-forward the local k3d loop. |
| Initial volumes | K3s Local Path Provisioner | Node-local learning-environment persistence. |
| Cluster ingress | Traefik (K3s bundled) | Route tunnel traffic to Kubernetes services; exact origin path remains proposed. |
| Source and CI | GitHub + GitHub Actions | Public source/reviews and hosted CI within the zero-spend limit. |
| Image registry | GHCR | Public versioned FlowSpace service images. |
| GitOps reconciler | Argo CD | Pull desired Kubernetes state from Git. |
| Application manifests | Kustomize | Shared base with local, staging, and production overlays. |
| Infrastructure packaging | Helm | Reuse maintained, pinned third-party charts. |
| Integration tests | Testcontainers for Go | Disposable real dependencies for service-level tests. |
| API smoke tests | Postman + Postman CLI | Author and automate public API smoke collections. |
| Load tests | k6 | Controlled load experiments with explicit thresholds. |
| Go static analysis | golangci-lint | Repository-wide formatting and selected analyzers. |
| Dependency updates | Renovate | Propose reviewed dependency-update pull requests; no automerge. |
| Public edge | Cloudflare Tunnel + Access | Outbound application ingress and the staging admission gate. |
| Kubernetes secrets | Sealed Secrets | Store encrypted secret manifests for in-cluster decryption. |
| Instrumentation | OpenTelemetry | Vendor-neutral telemetry and context propagation. |
| Telemetry collection | Grafana Alloy | Collect and forward application and platform telemetry. |
| Metrics | Prometheus | Self-hosted metrics storage and queries. |
| Logs | Loki | Self-hosted centralized logs. |
| Traces | Tempo | Self-hosted distributed traces. |
| Dashboards | Grafana | Query and visualize metrics, logs, and traces. |
