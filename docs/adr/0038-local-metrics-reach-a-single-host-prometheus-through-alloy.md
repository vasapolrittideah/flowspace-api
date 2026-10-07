# ADR-0038: Local metrics reach a single-host Prometheus through Alloy

Date: 2026-10-02

Status: Accepted

## Context

Developers need to see which service fails or slows down, whether events wait in the outbox or the broker, and whether the telemetry stack drops data or runs out of disk. Logs and traces show single requests, but they cannot show rates, percentiles, or growth over time.

Identity defines one counter for session checks, but no meter provider exports it. Identity writes the outbox age and the broker lag as log lines, not as metrics. Workspace has no metrics. Kubernetes reports pod restarts and volume use only through cluster components that no service reads now.

The local-path provisioner in k3d creates `local` persistent volumes on the host filesystem. The kubelet reports their usage, but the provisioner does not enforce the requested size. Each volume reports the usage of the whole host filesystem.

[ADR-0037](0037-local-logs-and-traces-use-disposable-single-host-stores.md) decides local logs and traces and leaves metric collection open. The [Open proposals](../architecture.md#open-proposals) section of the architecture lists it.

## Decision

In the `local` environment, services push metrics with OTLP over gRPC to Alloy, and Alloy writes them to one Prometheus replica with remote write.

Services use the same Alloy endpoint as spans. Each service exports metrics every 30 seconds with a periodic reader. An export waits at most 10 seconds. A failed export drops that interval and never blocks a request. Services do not open a `/metrics` endpoint.

Alloy also scrapes cluster metrics every 30 seconds and writes them to the same Prometheus. The targets are the kubelet of each node for volume usage, kube-state-metrics for pod restarts, and Alloy, Loki, Tempo, and Prometheus for their own health. kube-state-metrics runs as one replica in the `local` overlay.

Prometheus runs as one replica on a node-local persistent volume of 5Gi. It accepts remote writes and scrapes no target itself. It keeps metrics for 7 days or 4GB, whichever limit it reaches first. The size limit bounds the disk use because the provisioner does not enforce the volume size. Its data is disposable under [ADR-0022](0022-single-host-storage-holds-disposable-data.md).

Grafana reads Prometheus through a data source that Git provisions, as it reads Loki and Tempo. Git also provisions the dashboards. Prometheus has no tunnel route, and developers reach it only through Grafana or `kubectl port-forward`.

## Alternatives Considered

### Each service exposes a scrape endpoint

- Pros: Prometheus scrapes directly, and a developer can read current values with one HTTP request.
- Cons: every service opens another port with its own NetworkPolicy, and pod discovery needs scrape configuration for each workload.
- Rejected: OTLP push reuses the exporter, endpoint, and NetworkPolicy that spans already use.

### Prometheus scrapes cluster targets itself

- Pros: Prometheus works without Alloy for cluster metrics.
- Cons: two components hold scrape configuration, and Prometheus needs read access to the Kubernetes API.
- Rejected: Alloy already reads pod logs from the Kubernetes API, so one collector keeps all collection rules.

### Read pod restarts without kube-state-metrics

- Pros: no new component.
- Cons: the kubelet and cAdvisor do not report a restart count for each container, so a query must guess restarts from changes in container start times.
- Rejected: kube-state-metrics reports the restart count directly and needs one small replica.

## Consequences

- Metrics in `local` are lost when the volume, the cluster, or the Docker Desktop data is deleted.
- Metrics for an interval are lost when Alloy or Prometheus is unavailable during that export.
- Volume usage from the kubelet shows the host filesystem, not one store. The dashboards cannot show how full a single store is from that value.
- Prometheus accepts any series that Alloy writes, so services must keep metric labels bounded.
- Alert routing and the telemetry settings of staging and production stay open proposals.
