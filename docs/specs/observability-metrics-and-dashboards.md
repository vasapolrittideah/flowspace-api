# Spec: Observability metrics and dashboards

Module id: `observability-metrics-and-dashboards`

Status: Approved

## Objective

Give developers who run Flowspace in the `local` cluster metrics and dashboards that show the health of the services over time. The first users are these developers, and all data is disposable under [ADR-0022](../adr/0022-single-host-storage-holds-disposable-data.md).

The capability must answer these questions from metrics alone:

1. Which service returns errors or answers slowly now? The developer sees the request rate, error rate, and latency percentiles of each service and route, and the pod restarts.
2. Do emails wait in the outbox or the broker? The developer sees the age of the oldest unpublished outbox event, the consumer lag, and the email delivery results.
3. Is the telemetry stack near full or dropping data? The developer sees the failed and refused telemetry in Alloy and the disk use of the host and of Prometheus.

Logs and traces show one request at a time. This capability adds rates, percentiles, and trends, and it turns the outbox age and broker lag log lines into metrics.

## Scope, dependencies, and ADRs

Depends on: `observability-logs-and-traces`.

The scope covers metrics from the `identity-api`, `identity-worker`, and `workspace-api` processes, and three dashboards in Grafana. It also covers Prometheus, kube-state-metrics, and the Alloy scrape configuration in the `local` overlay. This capability uses the OTLP exporter configuration, Alloy, and Grafana that `observability-logs-and-traces` adds.

This capability excludes these items:

- Alerts and runbooks, which `observability-alerts-and-runbooks` covers.
- The `staging` and `production` environments.
- Metrics from migration jobs.
- Metrics inside PostgreSQL, Redpanda, Keycloak, and Mailpit, such as query statistics or broker partition metrics.
- Authorization-watermark age and backup age. The system does not have these values yet.
- Spans and metric points that the OpenTelemetry SDK drops inside a process before they reach Alloy.

These decisions apply:

- [ADR-0010](../adr/0010-cap-ordinary-unary-requests-at-five-seconds.md): ordinary requests stop at five seconds, so the latency buckets cover that cap.
- [ADR-0017](../adr/0017-outbox-and-idempotent-consumers-deliver-events.md): the outbox and its consumers deliver events, so their age and lag show delivery delay.
- [ADR-0022](../adr/0022-single-host-storage-holds-disposable-data.md): metrics stored on the single host are disposable.
- [ADR-0028](../adr/0028-telemetry-is-vendor-neutral-and-correlated.md): services emit vendor-neutral metrics with the same service and environment identity as their logs and traces.
- [ADR-0030](../adr/0030-telemetry-is-bounded-and-non-blocking.md): metric labels are bounded, and export never blocks requests.
- [ADR-0038](../adr/0038-local-metrics-reach-a-single-host-prometheus-through-alloy.md): the collection path, export interval, scrape targets, Prometheus storage, retention, and access for `local`.

## Contract

### Metric names in Prometheus

Services name their instruments in OpenTelemetry form. Alloy converts each name for Prometheus: dots become underscores, the unit becomes a suffix such as `_seconds`, and a counter gets the `_total` suffix. For example, `http.server.request.duration` becomes `http_server_request_duration_seconds`.

Each service series carries a `service` label with the `service.name` resource value and an `environment` label with the `deployment.environment.name` resource value. These are the values in the log records of `observability-logs-and-traces`.

### Service metrics

| Instrument | Type and unit | Processes | Attributes |
| --- | --- | --- | --- |
| `http.server.request.duration` | Histogram, `s` | `identity-api`, `workspace-api` | `http.request.method`, `http.route`, `http.response.status_code` |
| `rpc.server.call.duration` | Histogram, `s` | `identity-api` public and internal listeners, `workspace-api` | `rpc.method`, `rpc.grpc.status_code` |
| `rpc.client.call.duration` | Histogram, `s` | `workspace-api` calls to `CheckSession` | `rpc.method`, `rpc.grpc.status_code` |
| `db.client.connection.count` | Gauge, `{connection}` | All three processes | `db.client.connection.state` with `idle` or `used` |
| `db.client.connection.max` | Gauge, `{connection}` | All three processes | None |
| `flowspace.db.connection.acquire_wait` | Counter, `s` | All three processes | None |
| `flowspace.db.connection.empty_acquires` | Counter, `{acquire}` | All three processes | None |
| `identity.outbox.oldest_age` | Gauge, `s` | `identity-worker` | None |
| `identity.broker.consumer_lag` | Gauge, `{record}` | `identity-worker` | None |
| `identity.email.deliveries` | Counter, `{delivery}` | `identity-worker` | `identity.email.kind`, `outcome` |

The attribute values come from these fixed sets:

- `http.request.method` is a known HTTP method or `_OTHER`.
- `http.route` is a route template from the public API, such as `/v1/workspaces/{workspace_id}`, or `unmatched` for a path that no route matches.
- `http.response.status_code` and `rpc.grpc.status_code` are the status that the server returns.
- `rpc.method` is the full gRPC method name of a registered method, or `unknown`.
- `identity.email.kind` is the code purpose of a code email, `verify-email`, `claim-account`, or `password-reset`, or `password-change-notice` for the notice after a password change.
- `outcome` is `delivered`, `failed`, or `commit_failed`.

The latency histograms use these bucket bounds in seconds: 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, and 10.

`flowspace.db.connection.acquire_wait` is the total time that callers waited for a connection. `flowspace.db.connection.empty_acquires` counts acquires that had to wait because no idle connection existed.

### Cluster and stack metrics

Alloy scrapes these signals. The plan names the exact metric names of each component version.

| Signal | Source | Bounded labels |
| --- | --- | --- |
| Container restart count | kube-state-metrics | `namespace`, `pod`, `container` |
| Used and total bytes of each volume | Kubelet | `namespace`, `persistentvolumeclaim` |
| Spans and metric points that Alloy refuses or fails to send | Alloy | Component name |
| Log entries that Alloy drops or fails to send | Alloy | Component name |
| Storage size of the Prometheus database | Prometheus | None |
| Ingestion failures of Loki, Tempo, and Prometheus | Each component | Component name |

kube-state-metrics reports only the Flowspace namespace and only the container restart family. Alloy drops other kube-state-metrics families before it writes to Prometheus.

### Dashboards

Git provisions three dashboards in a `Flowspace` folder. Each dashboard opens with a time range of the last hour and has a `service` variable where it applies.

| Dashboard | Panels |
| --- | --- |
| Service health | Request rate, error rate by status class, and p50, p95, and p99 latency for each HTTP route and gRPC method. `CheckSession` client latency and errors. Database connection use against the maximum and the acquire wait rate. Container restarts in the last hour. |
| Event delivery | Oldest unpublished outbox event age, consumer lag, and email deliveries by kind and outcome. |
| Telemetry stack | Refused, failed, and dropped telemetry in Alloy. Ingestion failures of Loki, Tempo, and Prometheus. Host filesystem use from the kubelet and the Prometheus database size. |

A status class groups status codes, such as `5xx` for HTTP or the gRPC codes that `observability-logs-and-traces` treats as errors. Error panels count the HTTP 5xx statuses and those gRPC codes.

## Behavior

### Measurement

- Record the server duration of each request after the response status is known, including requests that fail authentication or validation.
- Record `rpc.server.call.duration` on the Identity internal listener for each `CheckSession` call. Remove the `identity.session_checks` counter because this histogram counts the same calls.
- Read the database pool values and the outbox age at each export. The outbox age is the age of the oldest event with no `published_at`, or 0 when no such event exists.
- Read the consumer lag at each export for the email worker consumer group.
- If a measurement fails or takes longer than 5 seconds, omit that value for the interval and write the existing `identity_outbox_age_unavailable` or `identity_broker_lag_unavailable` line.
- Remove the `identity_outbox_age` and `identity_broker_lag` info log lines. The metrics replace them.

### Bounded and non-blocking export

Apply ADR-0038 to each process:

- Export metrics every 30 seconds with a periodic reader. Limit each export to 10 seconds.
- If an export fails, drop the values of that interval. A request never waits for a metric export.
- If `OTEL_EXPORTER_OTLP_ENDPOINT` is not set, the process starts and exports no metrics.
- On shutdown, export the last interval within the existing shutdown time, then stop.

### Collection and storage

- Alloy receives service metrics on its existing OTLP endpoint and writes them to Prometheus with remote write.
- Alloy scrapes the kubelet, kube-state-metrics, Alloy, Loki, Tempo, and Prometheus every 30 seconds.
- Prometheus runs one replica on a 5Gi node-local volume and keeps metrics for 7 days or 4GB, whichever limit it reaches first.
- Prometheus and kube-state-metrics have CPU and memory requests and limits.
- Git provisions the Prometheus data source in Grafana.

### Security and abuse

- NetworkPolicy allows the Prometheus port only from Alloy and Grafana, and the kube-state-metrics port only from Alloy.
- The Alloy service account can `get` the `nodes/metrics` resource to read the kubelet. It cannot use `nodes/proxy`.
- Prometheus and kube-state-metrics have no tunnel route. Developers reach Prometheus only through Grafana or `kubectl port-forward`.

### Diagnostics

The capability records the metrics in the contract. Metric names, attribute values, and labels must never contain these values:

- User IDs, subject IDs, session IDs, workspace IDs, request IDs, or trace IDs.
- Email addresses, provider account names, tokens, passwords, or codes.
- Raw URL paths, query strings, or error message text.

## Testing strategy

| Risk | Test level | Environment |
| --- | --- | --- |
| A request waits for a metric export, or a process fails to start without Alloy | Unit | None |
| A process stops serving requests when Alloy is stopped in the cluster | Cluster | Local cluster |
| An HTTP or gRPC duration has a raw path, an unknown method name, or a missing status | Unit | None |
| Workspace records no `CheckSession` client duration, or Identity records no internal server duration | Unit | None |
| The outbox age is wrong when events are unpublished, published, or absent | Integration | Docker |
| A failed outbox age or lag measurement blocks export or reports a stale value | Unit | None |
| Email delivery results have the wrong kind or outcome | Unit | None |
| A metric label takes an unbounded value | Cluster | Local cluster |
| kube-state-metrics writes families or namespaces outside the allowlist | Cluster | Local cluster |
| Prometheus or kube-state-metrics is reachable from a pod other than Alloy or Grafana | Cluster | Local cluster |
| A dashboard panel shows no data for traffic that exists | Cluster | Local cluster |

## Implementation boundaries

### Always

- Use the OpenTelemetry semantic convention names for HTTP, RPC, and database connection instruments.
- Add each new component and dashboard to the `local` overlay so that Tilt applies it with the services.
- Keep dashboards as JSON files in Git, and do not save changes from the Grafana UI as the source.
- Record the measured CPU and memory use of Prometheus and kube-state-metrics in the plan after the first full run.

### Ask first

- Change an export interval, scrape interval, retention, or volume value from ADR-0038.
- Add a metric attribute, a metric that this specification does not name, or a kube-state-metrics family.
- Add a dashboard or a telemetry component that this specification does not name.

### Never

- Do not open a `/metrics` endpoint in a service.
- Do not add alert rules or notification channels.
- Do not add telemetry components to `staging` or `production` overlays or to the tunnel configuration.

## Success criteria

1. Given the local cluster runs the stack, When a client sends requests to each public route of `identity-api` and `workspace-api`, Then within 90 seconds Prometheus has `http_server_request_duration_seconds` series for those routes with `service`, `environment`, `http_route`, and status labels.
2. Given an authenticated client reads a workspace, When the request completes, Then Prometheus has a `CheckSession` client duration from `workspace-api` and a `CheckSession` server duration from `identity-api`.
3. Given a client sends a request to a path that no route matches, When the service answers, Then the series has the `unmatched` route, and no series has the raw path.
4. Given a request fails with HTTP 5xx or a gRPC error code, When a developer opens the Service health dashboard, Then the error rate panel shows the failure for that service and route.
5. Given requests with known latencies, When a developer opens the Service health dashboard, Then it shows p50, p95, and p99 latency for each route from the histogram buckets.
6. Given unpublished events wait in the outbox because the relay is stopped, When a developer opens the Event delivery dashboard, Then the outbox age increases until the relay publishes them, and then it returns to 0.
7. Given the email worker is stopped while events are published, When a developer opens the Event delivery dashboard, Then the consumer lag increases until the worker consumes the records.
8. Given a signup sends a verification email, When the worker delivers it, Then `identity_email_deliveries_total` increases for kind `verify-email` and outcome `delivered`.
9. Given a Flowspace container restarts, When a developer opens the Service health dashboard, Then the restart panel shows the restart for that pod and container.
10. Given the stack runs, When a developer opens the Telemetry stack dashboard, Then it shows host filesystem use, the Prometheus database size, and the Alloy refused and failed counts.
11. Given Alloy is stopped, When clients send requests to each API, Then each request returns its normal result within its normal deadline, and each process keeps running.
12. Given the stack has run for a test period, When a developer lists the label names and values in Prometheus, Then no value is a user, subject, session, workspace, request, or trace ID, an email address, a raw path, or error text.
13. Given the stack runs, When a developer lists the kube-state-metrics series in Prometheus, Then only container restart series from the Flowspace namespace exist.
14. Given the stack runs in `local`, When a developer lists the routes of the tunnel and tries to connect to Prometheus from a pod other than Alloy or Grafana, Then no route exists and the connection fails.
15. Given the stack runs in `local`, When a developer reads the Prometheus pod, volume, and flags, Then it has one replica with a bound 5Gi volume and retention of 7 days and 4GB.
16. Given a developer opens Grafana, When the developer lists the `Flowspace` folder, Then it contains the Service health, Event delivery, and Telemetry stack dashboards from Git.
