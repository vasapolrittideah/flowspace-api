# Spec: Observability metrics and dashboards

Module ID: `observability-metrics-and-dashboards`

Status: Draft

## Objective

Give developers who run Flowspace in the `local` cluster metrics and dashboards that show the health of the services over time. The first users are these developers, and all data is disposable under [ADR-0022](../adr/0022-single-host-storage-holds-disposable-data.md).

The capability must answer three questions from metrics alone. First, which service returns errors or answers slowly now? The developer sees the request rate, error rate, and latency percentiles of each service and route, and the pod restarts. Second, do emails wait in the outbox or the broker? The developer sees the age of the oldest unpublished outbox event, the consumer lag, and the email delivery results. Third, is the telemetry stack near full or dropping data? The developer sees the failed and refused telemetry in Alloy and the disk use of the host and of Prometheus.

Logs and traces show one request at a time. This capability adds rates, percentiles, and trends, and it turns the outbox age and broker lag log lines into metrics.

## Scope and ADRs

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

Services name their instruments in OpenTelemetry form, and Alloy converts each name for Prometheus. Dots become underscores, the unit becomes a suffix such as `_seconds`, a unit in braces such as `{connection}` adds no suffix, and a counter gets the `_total` suffix. Each service series also carries the `service` and `environment` labels.

The latency histograms use these bucket bounds in seconds: 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, and 10.

### Metrics

| Metric | Type and unit | Source | Labels | Instrument | Meaning |
| --- | --- | --- | --- | --- | --- |
| `http_server_request_duration_seconds` | Histogram, seconds | `identity-api`, `workspace-api` | `http_request_method`, `http_route`, `http_response_status_code` | `http.server.request.duration` | Duration of each HTTP request |
| `rpc_server_call_duration_seconds` | Histogram, seconds | `identity-api` public and internal listeners, `workspace-api` | `rpc_method`, `rpc_grpc_status_code` | `rpc.server.call.duration` | Duration of each gRPC call that the server handles |
| `rpc_client_call_duration_seconds` | Histogram, seconds | `workspace-api` calls to `CheckSession` | `rpc_method`, `rpc_grpc_status_code` | `rpc.client.call.duration` | Duration of each `CheckSession` call from Workspace |
| `db_client_connection_count` | Gauge, connections | All three processes | `db_client_connection_state` | `db.client.connection.count` | Connections in the database pool |
| `db_client_connection_max` | Gauge, connections | All three processes | None | `db.client.connection.max` | Maximum size of the database pool |
| `flowspace_db_connection_acquire_wait_seconds_total` | Counter, seconds | All three processes | None | `flowspace.db.connection.acquire_wait` | Total time that callers waited for a connection |
| `flowspace_db_connection_empty_acquires_total` | Counter, acquires | All three processes | None | `flowspace.db.connection.empty_acquires` | Acquires that waited because no idle connection existed |
| `identity_outbox_oldest_age_seconds` | Gauge, seconds | `identity-worker` | None | `identity.outbox.oldest_age` | Age of the oldest unpublished outbox event |
| `identity_broker_consumer_lag` | Gauge, records | `identity-worker` | None | `identity.broker.consumer_lag` | Records that the email worker consumer group has not consumed |
| `identity_email_deliveries_total` | Counter, deliveries | `identity-worker` | `identity_email_kind`, `outcome` | `identity.email.deliveries` | Email delivery attempts |

### Metric label values

| Label | Values |
| --- | --- |
| `service` | The `service.name` resource value, which is the `service` value in the log records of `observability-logs-and-traces` |
| `environment` | The `deployment.environment.name` resource value, which is the `environment` value in the log records |
| `http_request_method` | A known HTTP method, or `_OTHER` |
| `http_route` | A route template from the public API, such as `/v1/workspaces/{workspace_id}`, or `unmatched` for a path that no route matches |
| `http_response_status_code` | The HTTP status that the server returns |
| `rpc_method` | The full gRPC method name of a registered method, or `unknown` |
| `rpc_grpc_status_code` | The gRPC status that the server returns |
| `db_client_connection_state` | `idle` or `used` |
| `identity_email_kind` | `verify-email`, `claim-account`, or `password-reset` for the code purpose of a code email, or `password-change-notice` for the notice after a password change |
| `outcome` | `delivered`, `failed`, or `commit_failed` |

### Scraped signals

The plan names the exact metric names of each component version.

| Signal | Source | Labels |
| --- | --- | --- |
| Container restart count | kube-state-metrics | `namespace`, `pod`, `container` |
| Used and total bytes of each volume | Kubelet | `namespace`, `persistentvolumeclaim` |
| Spans and metric points that Alloy refuses or fails to send | Alloy | Component name |
| Log entries that Alloy drops or fails to send | Alloy | Component name |
| Storage size of the Prometheus database | Prometheus | None |
| Ingestion failures of Loki, Tempo, and Prometheus | Each component | Component name |

### Dashboards

Git provisions three dashboards in a `Flowspace` folder. Each dashboard opens with a time range of the last hour and has a `service` variable where it applies.

| Dashboard | UID | Panels |
| --- | --- | --- |
| Service health | `flowspace-service-health` | Request rate, Error rate, Latency percentiles, CheckSession latency, CheckSession errors, Database connections, Connection acquire wait, Container restarts |
| Event delivery | `flowspace-event-delivery` | Oldest outbox event age, Consumer lag, Email deliveries |
| Telemetry stack | `flowspace-telemetry-stack` | Alloy telemetry loss, Store ingestion failures, Host filesystem use, Prometheus database size |

The Service health panels show these values for each HTTP route and gRPC method: the request rate, the error rate by status class, and the p50, p95, and p99 latency. A status class groups status codes, such as `5xx` for HTTP or the gRPC codes that `observability-logs-and-traces` treats as errors. Error panels count the HTTP 5xx statuses and those gRPC codes. The CheckSession panels show the client latency and errors. Database connections shows the connection use against the maximum, and Container restarts shows the restarts in the last hour.

Email deliveries shows the deliveries by kind and outcome. Alloy telemetry loss shows the refused, failed, and dropped telemetry in Alloy. Host filesystem use reads the kubelet volume metrics.

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
- kube-state-metrics reports only the Flowspace namespace and only the container restart family. Alloy drops other kube-state-metrics families before it writes to Prometheus.
- Prometheus runs one replica on a 5Gi node-local volume and keeps metrics for 7 days or 4GB, whichever limit it reaches first.
- Prometheus and kube-state-metrics have CPU and memory requests and limits.
- Git provisions the Prometheus data source in Grafana.

### Security and abuse

- NetworkPolicy allows the Prometheus port only from Alloy and Grafana, and the kube-state-metrics port only from Alloy.
- The Alloy service account can `get` the `nodes/metrics` resource to read the kubelet. It cannot use `nodes/proxy`.
- Prometheus and kube-state-metrics have no tunnel route. Developers reach Prometheus only through Grafana or `kubectl port-forward`.

### Diagnostics

Metric names, attribute values, and labels must never contain these values:

- User IDs, subject IDs, session IDs, workspace IDs, request IDs, or trace IDs.
- Email addresses, provider account names, tokens, passwords, or codes.
- Raw URL paths, query strings, or error message text.

## Testing strategy

| Risk | Test level |
| --- | --- |
| A request waits for a metric export, or a process fails to start without Alloy | Unit |
| A process stops serving requests when Alloy is stopped in the cluster | Cluster |
| An HTTP or gRPC duration has a raw path, an unknown method name, or a missing status | Unit |
| Workspace records no `CheckSession` client duration, or Identity records no internal server duration | Unit |
| The outbox age is wrong when events are unpublished, published, or absent | Integration |
| A failed outbox age or lag measurement blocks export or reports a stale value | Unit |
| Email delivery results have the wrong kind or outcome | Unit |
| A metric label takes an unbounded value | Cluster |
| kube-state-metrics writes families or namespaces outside the allowlist | Cluster |
| Prometheus or kube-state-metrics is reachable from a pod other than Alloy or Grafana | Cluster |
| A dashboard panel shows no data for traffic that exists | Cluster |

## Implementation boundaries

Always do these actions:

- Use the OpenTelemetry semantic convention names for HTTP, RPC, and database connection instruments.
- Add each new component and dashboard to the `local` overlay so that Tilt applies it with the services.
- Keep dashboards as JSON files in Git. Do not save changes from the Grafana UI as the source.

Ask the maintainer before these actions:

- Change an export interval, scrape interval, retention, or volume value from ADR-0038.
- Add a metric attribute, a metric that this specification does not name, or a kube-state-metrics family.
- Add a dashboard or a telemetry component that this specification does not name.

Never do these actions:

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
