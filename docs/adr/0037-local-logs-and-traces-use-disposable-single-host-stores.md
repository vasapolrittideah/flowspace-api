# ADR-0037: Local logs and traces use disposable single-host stores

Date: 2026-10-01

Status: Accepted

## Context

[ADR-0028](0028-telemetry-is-vendor-neutral-and-correlated.md), [ADR-0029](0029-the-observability-stack-is-self-hosted.md), and [ADR-0030](0030-telemetry-is-bounded-and-non-blocking.md) set the telemetry direction but leave collection paths, storage modes, retention, and sampling open. The [architecture](../architecture.md#open-proposals) keeps these items as open proposals until an ADR accepts them.

Identity and Workspace already write JSON logs to standard output and extract W3C trace context from HTTP requests. Identity also creates spans and carries trace context through event headers. No exporter sends these spans anywhere, so the SDK drops them. The current sampler keeps 10% of root traces, so a developer often cannot find the trace for one test request.

Only the `local` overlay exists. It runs in a lightweight Kubernetes cluster inside Docker Desktop on Mac, as [ADR-0021](0021-run-kubernetes-locally-and-on-the-self-hosted-server.md) describes.

## Decision

This decision applies to logs and traces in the `local` environment. Metrics, dashboards, and alerts come later, when a service emits those signals for a specific need.

Services keep writing JSON logs to standard output. Grafana Alloy reads pod logs through the Kubernetes API and sends them to Loki. Services do not export logs through OpenTelemetry. A log line connects to its trace through its `trace_id` field.

Services export spans with OTLP over gRPC to Alloy inside the cluster. Alloy sends the spans to Tempo. Each service uses a batch span processor with a bounded queue of 2048 spans. When the queue is full, the processor drops new spans. Export waits at most 10 seconds and never blocks a request.

The sampler follows the parent decision. For root spans, it uses a ratio that each environment sets in its configuration. The `local` ratio is 1.0, so every local trace is kept. Each later environment sets its own ratio when its overlay is added.

Loki and Tempo each run as one single-binary replica on a node-local persistent volume of 5Gi. Their data is disposable, as [ADR-0022](0022-single-host-storage-holds-disposable-data.md) states. Loki keeps logs for 7 days. Tempo keeps traces for 3 days. Each component has CPU and memory limits in its manifest.

Grafana reads Loki and Tempo through data sources that Git provisions. Grafana has no persistent volume. Developers open Grafana only with `kubectl port-forward`. The outbound tunnel in [ADR-0026](0026-an-outbound-tunnel-exposes-selected-routes.md) has no route to Grafana, Loki, Tempo, or Alloy.

## Alternatives Considered

### Export logs through OpenTelemetry

- Pros: one export path for logs and traces, and the SDK adds trace IDs to each record.
- Cons: every service needs a log bridge, and a failed collector can hide the only copy of a log line.
- Rejected: standard output already keeps logs in `kubectl logs` when the collector fails, and Alloy can read them without application changes.

### Keep the 10% trace sample in local

- Pros: less trace storage on the developer's machine.
- Cons: most single test requests have no trace, so a developer cannot follow one request.
- Rejected: local traffic is low, and the volume size and retention limit storage.

### Use object storage for Loki and Tempo

- Pros: it matches a scalable deployment and separates storage from the store processes.
- Cons: it adds another stateful workload on the same host and does not protect data from host loss.
- Rejected: the data is disposable, so the filesystem is enough.

### Expose Grafana through the tunnel

- Pros: developers can open dashboards without cluster credentials.
- Cons: it adds a public route that needs its own authentication and access rules.
- Rejected: port forwarding uses the existing cluster access and adds no public surface.

## Consequences

- Telemetry in `local` is lost when the volume, the cluster, or the Docker Desktop data is deleted.
- When a volume fills before its retention ends, the store rejects new data. Requests continue, and telemetry for that period is lost.
- Spans are lost when Alloy or Tempo is unavailable for longer than the queue can hold.
- Staging and production need a new decision for sampling, retention, volume size, and access before their overlays are added.
- Metric collection and alert routing stay open proposals.
