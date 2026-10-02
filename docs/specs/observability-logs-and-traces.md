# Spec: Observability logs and traces

Module id: `observability-logs-and-traces`

Status: Approved

## Objective

Give developers who run Flowspace in the `local` cluster one place to find the logs and traces of a request. The first users are these developers, and all data is disposable under [ADR-0022](../adr/0022-single-host-storage-holds-disposable-data.md). A developer starts from a request ID or a trace ID and follows the request through each service and event that it touches.

The capability must answer these questions from telemetry alone:

1. Why did this request fail? The developer finds the logs of a request by its request ID and opens its trace.
2. Where did a slow request spend its time? The trace shows the time in Workspace and in its session check to Identity.
3. Why did a signup email not arrive? One trace connects the signup request, the outbox publish, and the email worker.

Services already write JSON logs and create some spans, but no collector stores the logs and no exporter sends the spans. This capability makes those signals reach storage and fills the gaps in trace context that stop the three questions above.

## Scope and decisions

Depends on: none.

The scope covers the `identity-api`, `identity-worker`, and `workspace-api` processes. It also covers Alloy, Loki, Tempo, and Grafana in the `local` overlay, which Tilt applies. Migration jobs are in scope for logs only. Their logs reach Loki through the same pod log path, and they create no spans.

This capability excludes these items:

- Metrics and dashboards, which `observability-metrics-and-dashboards` covers, and alerts, which `observability-alerts-and-runbooks` covers.
- The `staging` and `production` environments. They have no overlay yet.
- Database client spans and spans inside use cases.
- Logs and traces from platform workloads such as PostgreSQL, Redpanda, Keycloak, and Mailpit.
- Tail sampling and log export through OpenTelemetry.

These decisions apply:

- [ADR-0022](../adr/0022-single-host-storage-holds-disposable-data.md): telemetry stored on the single host is disposable.
- [ADR-0026](../adr/0026-an-outbound-tunnel-exposes-selected-routes.md): the outbound tunnel exposes only selected routes, so telemetry components get no route.
- [ADR-0027](../adr/0027-git-contains-only-encrypted-kubernetes-secrets.md): the Grafana admin password is an encrypted Kubernetes secret in Git.
- [ADR-0028](../adr/0028-telemetry-is-vendor-neutral-and-correlated.md): services emit vendor-neutral telemetry and carry trace context across synchronous calls and events.
- [ADR-0029](../adr/0029-the-observability-stack-is-self-hosted.md): the stack is self-hosted and grows one component at a time.
- [ADR-0030](../adr/0030-telemetry-is-bounded-and-non-blocking.md): telemetry is bounded and never blocks requests.
- [ADR-0036](../adr/0036-authenticate-internal-session-checks-with-mutual-tls.md): the session check uses a separate mutual TLS listener.
- [ADR-0037](../adr/0037-local-logs-and-traces-use-disposable-single-host-stores.md): collection paths, storage, retention, sampling, and Grafana access for `local`.

## Contract

### Log records

Each application log line is one JSON object on standard output. The `msg` field holds a stable event name in `snake_case`, such as `identity_request` or `email_delivery_failed`.

| Field | Required on | Meaning |
| --- | --- | --- |
| `ts`, `level`, `msg` | Every line | Time, level, and event name from the shared logger |
| `service` | Every line | Process name, such as `identity-api` |
| `environment` | Every line | Environment name, such as `local` |
| `request_id` | Request lines | Effective `X-Request-ID` of an inbound HTTP request, or the forwarded `x-request-id` gRPC metadata |
| `trace_id` | Request and event lines | 32-character lowercase hex trace ID when the context has a valid span |
| `operation` | Request and event lines | Bounded operation name, such as a route template or a full gRPC method name |
| `outcome`, `status`, `duration` | Completion lines | Result, HTTP or gRPC status, and elapsed time |

A request line is a line that a service writes while it handles one inbound request. An event line is a line that the worker or the outbox relay writes while it handles one event.

Loki stores `service`, `environment`, and `namespace` as labels. All other fields, including `request_id` and `trace_id`, stay in the log line. A developer filters them with a LogQL JSON parser. Loki never stores a request ID, trace ID, subject, or path as a label.

### Trace resources and spans

Each process sets the resource attribute `service.name` to its `service` log value and `deployment.environment.name` to its `environment` log value.

| Process | Span | Kind | Parent |
| --- | --- | --- | --- |
| `identity-api` | One span for each public HTTP or gRPC request | Server | Incoming `traceparent`, or a new root |
| `identity-api` | One span for each internal `CheckSession` call | Server | `traceparent` in gRPC metadata |
| `workspace-api` | One span for each public HTTP or gRPC request | Server | Incoming `traceparent`, or a new root |
| `workspace-api` | One span for each `CheckSession` call to Identity | Client | The Workspace request span |
| `identity-worker` | `identity.outbox_publish` for each published outbox event | Producer | The trace context stored with the outbox event |
| `identity-worker` | `identity.email_delivery` for each delivered email event | Consumer | `traceparent` in the event record headers |
| `identity-worker` | `identity.password_change_notice` for each delivered notice | Consumer | A new root, because the notice comes from a database row and not from an event record |

A server span name is a bounded operation name, such as the HTTP method and route template, or the full gRPC method name. A span name never contains an ID or a raw path.

A server span records `http.request.method`, `http.route`, and `http.response.status_code` for HTTP. It records `rpc.method` and `rpc.grpc.status_code` for gRPC. A span has the error status when the HTTP status is 5xx or the gRPC code is `Internal`, `Unavailable`, `DeadlineExceeded`, or `Unknown`.

### Trace context propagation

The Workspace `CheckSession` call carries `traceparent`, `tracestate` when present, and `x-request-id` as gRPC metadata. Identity reads them on its internal listener.

Each Identity event record carries the W3C `traceparent` and, when present, `tracestate` headers of the request or relay span that produced it.

### Configuration

Each process reads standard OpenTelemetry environment variables. `OTEL_EXPORTER_OTLP_ENDPOINT` names the Alloy OTLP gRPC endpoint. `OTEL_TRACES_SAMPLER` is `parentbased_traceidratio`, and `OTEL_TRACES_SAMPLER_ARG` is `1.0` in `local`.

## Behavior

### Correlation

Apply these rules in each process:

- Write `trace_id` on every request and event line when the context has a valid span. This includes `identity_rpc`, `identity_session_check`, and the Workspace `request_completed` line, which do not have it now.
- Write the same `trace_id` in the log line and in the span of one request.
- Write the request ID that Workspace forwards on the Identity `identity_session_check` line.
- Keep the request ID rules of the [architecture](../architecture.md#observability-and-recovery). Do not replace a valid request ID with a trace ID.

### Context in the outbox

The Identity outbox keeps the `traceparent` and `tracestate` of the transaction that inserts each event. The relay continues that context when it publishes the event, and the publisher writes it to the record headers. An event that has no stored context starts a new root trace in the relay.

### Bounded and non-blocking export

Apply ADR-0037 to each process:

- Use a batch span processor with a queue of 2048 spans. When the queue is full, drop new spans.
- Limit each export to 10 seconds. A request never waits for an export.
- If `OTEL_EXPORTER_OTLP_ENDPOINT` is not set, the process starts and exports no spans.
- If Alloy or Tempo is unavailable at startup or later, the process starts and serves requests. The process does not retry export without a limit.
- On shutdown, flush spans within the existing shutdown time, then stop. A flush that does not finish in time drops the remaining spans.

### Collection and storage

The `local` overlay adds Alloy, Loki, Tempo, and Grafana. Apply these rules:

- Alloy reads the logs of Flowspace application pods through the Kubernetes API, parses the JSON, sets the Loki labels, and sends the lines to Loki. Lines that are not JSON keep their text and labels.
- Alloy receives OTLP over gRPC on a cluster-only Service and sends the spans to Tempo.
- Loki and Tempo each run one single-binary replica on a 5Gi node-local volume. Loki keeps logs for 7 days, and Tempo keeps traces for 3 days.
- Each telemetry component has CPU and memory requests and limits.
- Git provisions the Loki and Tempo data sources in Grafana. A log line links to its trace through `trace_id`, and a trace links back to its logs. Grafana has no persistent volume.

### Security and abuse

- NetworkPolicy allows the Alloy OTLP port only from Flowspace application pods. Loki and Tempo accept connections only from Alloy and Grafana.
- Grafana has no anonymous access. Its admin password comes from a Sealed Secret under ADR-0027.
- Developers open Grafana only with `kubectl port-forward`. The tunnel has no route to any telemetry component.

### Data and compatibility

A migration adds nullable `traceparent` and `tracestate` text columns to `identity_outbox_events`. The migration only adds columns, so a running older relay keeps working, and rows that existed before the migration have no stored context. Rollback drops the two columns and loses only stored trace context.

### Diagnostics

The capability records the log fields, Loki labels, and spans in the contract. Logs, span names, span attributes, span events, and Loki labels must never contain these values:

- Access tokens, refresh tokens, provider tokens, and handoff values.
- Passwords, verification codes, reset codes, and claim codes.
- Email addresses and provider account names.
- `Authorization`, `Cookie`, and `Set-Cookie` headers.
- Request and response bodies, query strings, and client certificate data.

Subject IDs and session IDs are allowed in log fields only where a line already has them. They are not allowed as span attributes.

## Testing strategy

| Risk | Test level | Environment |
| --- | --- | --- |
| A process fails to start or serve requests when Alloy is missing or unreachable | Unit | None |
| A process fails to start or serve requests when Alloy is stopped in the cluster | Cluster | Local cluster |
| The sampler or resource attributes ignore the environment configuration | Unit | None |
| A server span ignores the incoming `traceparent`, has an unbounded name, or has a different `trace_id` from its log line | Unit | None |
| The Workspace `CheckSession` call drops `traceparent` or `x-request-id`, or Identity does not continue them | Unit | None |
| The outbox loses trace context at commit, or the relay and email worker do not continue it | Integration | Docker |
| A log line or span contains a token, password, code, or email address | Unit | None |
| A stored log line, span, or Loki label contains a token, password, code, or email address | Cluster | Local cluster |
| A request waits for span export | Unit | None |
| A Loki label takes unbounded values such as request IDs, trace IDs, or paths | Cluster | Local cluster |
| A telemetry component is reachable outside the cluster | Cluster | Local cluster |
| Logs and traces of one request cannot be found together in Grafana | Cluster | Local cluster |

## Implementation boundaries

### Always

- Add the outbox columns in a new migration file. Do not edit an existing migration.
- Extend the shared logging and request ID packages instead of adding a second logger or request ID helper.
- Add each telemetry component to the `local` overlay so that Tilt applies it with the services.
- Record the measured CPU and memory use of each telemetry component in the plan after the first full run.

### Ask first

- Change a retention, volume, sampling, or queue value from ADR-0037.
- Add a Loki label, a telemetry component that this specification does not name, or a route to Grafana.
- Change the outbox schema beyond the stored trace context.
- Add a Go dependency other than OpenTelemetry exporters and instrumentation.

### Never

- Do not change the public Identity or Workspace API contracts.
- Do not add telemetry components to `staging` or `production` overlays or to the tunnel configuration.

## Success criteria

1. Given the local cluster runs the stack, When a client sends a request to `identity-api` or `workspace-api` with a valid `X-Request-ID`, Then Grafana finds its log lines in Loki by that request ID, and each line has `service`, `environment`, `request_id`, and `trace_id`.
2. Given a request has a `trace_id` in its log line, When a developer opens that trace in Grafana, Then Tempo shows the server span of the service, and the log line links to the trace.
3. Given an authenticated client reads a workspace, When the request passes through Workspace and the Identity session check, Then one trace contains the Workspace server span, the Workspace `CheckSession` client span, and the Identity `CheckSession` server span. The Identity `identity_session_check` line has the same request ID and trace ID.
4. Given a client starts a signup that sends a verification email, When the relay publishes the event and the worker delivers the email, Then one trace contains the Identity request span, `identity.outbox_publish`, and `identity.email_delivery`.
5. Given a request has a `traceparent` header from the client, When the service handles the request, Then its server span is a child of that context.
6. Given Alloy is stopped, When clients send requests to each API, Then each request returns its normal result within its normal deadline. Each process keeps running and its queue stays at or below 2048 spans.
7. Given Alloy is stopped, When a process starts, Then the process becomes ready and serves requests.
8. Given requests carry known token, password, code, and email values, When their logs and traces are stored, Then no stored log line, span, or Loki label contains those values.
9. Given the stack has run for a test period, When a developer lists the Loki label names and values, Then the only labels are `service`, `environment`, `namespace`, and the labels that Loki adds itself, and no label value is a request ID, trace ID, or path.
10. Given the stack runs in `local`, When a developer lists the routes of the tunnel and the Services of the cluster, Then no telemetry component has a tunnel route, and Grafana answers only through `kubectl port-forward`.
11. Given the stack runs in `local`, When a developer lists the Loki and Tempo pods and volumes, Then each store has one running replica with a bound 5Gi volume, and its configured retention is 7 days for Loki and 3 days for Tempo.
