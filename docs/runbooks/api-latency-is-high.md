# Runbook: API latency is high

Alert: `API latency is high`

Severity: `ticket`

## Meaning

API clients wait long for the answers of 1 Flowspace service. The alert fires when, over the last 5 minutes, the p99 HTTP or gRPC server duration of a service is above 2.5 seconds and the service receives at least 0.1 requests each second. The condition must stay true for 10 minutes. The [`deploy/overlays/local/observability/alerting/rules.json`](../../deploy/overlays/local/observability/alerting/rules.json) file holds the query. An email with the `DatasourceError` alert name and this runbook means that Grafana cannot query Prometheus.

## First checks

1. Open Grafana with `kubectl -n flowspace-local port-forward svc/grafana 3000:80`, or use the Grafana port forward of Tilt. Then open the Latency percentiles panel of the [Service health](../../deploy/overlays/local/observability/dashboards/service-health.json) dashboard at `http://localhost:3000/d/flowspace-service-health`. Select the `service` label of the alert.
2. In Grafana Explore, run this query against the Prometheus data source to find the slow routes and methods:

   ```promql
   histogram_quantile(0.99, sum by (service, http_route, le) (rate(http_server_request_duration_seconds_bucket[5m])))
     or histogram_quantile(0.99, sum by (service, rpc_method, le) (rate(rpc_server_call_duration_seconds_bucket[5m])))
   ```

   If a route of `workspace-api` is at 5 seconds or more, go to the next check. Its requests reach the 5-second request timeout. If only 1 route is slow, read its traces in the fourth check.
3. Open the CheckSession latency and Connection acquire wait panels of the same dashboard. A high CheckSession latency points to Identity. A high acquire wait points to the database of the service.
4. In Grafana Explore, run this query against the Loki data source to find slow requests of the service:

   ```logql
   {service="workspace-api"} | json | msg="request_completed" | duration > 2.5
   ```

   Replace `workspace-api` with the `service` label of the alert. The `operation` and `status` fields name the slow call. Follow the `trace_id` of a line to its trace in Tempo. Then find the span that takes the most time.

## Resolution

### A database query waits for a lock

1. Find the queries that wait for a lock in the database of the service:

   ```bash
   kubectl -n flowspace-local exec deploy/workspace-postgres -- sh -c 'psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -c "SELECT pid, pg_blocking_pids(pid) AS blocked_by, application_name, state, wait_event_type, now() - xact_start AS age, left(query, 60) AS query FROM pg_stat_activity WHERE datname = current_database() ORDER BY age DESC NULLS LAST"'
   ```

   Replace `workspace-postgres` with `identity-postgres` for Identity.
2. Find the rows with `Lock` in the `wait_event_type` column. The `blocked_by` column of each row names the process IDs that hold the lock.
3. Read the `application_name` and `query` columns of each blocking process. If the process is a manual session and not a Flowspace service, run `SELECT pg_terminate_backend(<pid>)` in the same `psql` command. The transaction of that process rolls back, so its changes are lost.

The Latency percentiles panel shows the p99 under 2.5 seconds, and the alert sends a resolved email within 10 minutes.

### Identity answers slowly

1. Run `kubectl -n flowspace-local get pods -l app.kubernetes.io/name=identity-api`. Then make sure that the pod is ready and does not restart.
2. Follow the API error rate is high runbook in [`api-error-rate-is-high.md`](api-error-rate-is-high.md) for the Identity causes.

The CheckSession latency panel shows the p99 under 2.5 seconds.

### A code change made a call slow

1. Find the latest change to the service with `git log -- services/<service>`.
2. Revert the change on a branch. Then run `tilt up` to deploy it.

The Latency percentiles panel shows the p99 under 2.5 seconds after the new pod starts.

## Escalation

If the p99 stays above 2.5 seconds after these steps, open a GitHub Issue with the `type:fix` label. Attach the result of the second check, a screenshot of the panels of the third check, and 3 slow log lines with their trace IDs.
