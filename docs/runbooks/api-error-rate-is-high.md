# Runbook: API error rate is high

Alert: `API error rate is high`

Severity: `page`

## Meaning

API clients receive server errors from 1 Flowspace service. The alert fires when, over the last 5 minutes, more than 5% of the HTTP and gRPC server requests of a service end with an error status and the service receives at least 0.1 requests each second. The condition must stay true for 5 minutes. An error status is an HTTP 5xx status or the gRPC code `Unknown`, `DeadlineExceeded`, `Internal`, or `Unavailable`. The [`deploy/overlays/local/observability/alerting/rules.json`](../../deploy/overlays/local/observability/alerting/rules.json) file holds the query. An email with the `DatasourceError` alert name and this runbook means that Grafana cannot query Prometheus.

## First checks

1. Open Grafana with `kubectl -n flowspace-local port-forward svc/grafana 3000:80`, or use the Grafana port forward of Tilt. Then open the Error rate panel of the [Service health](../../deploy/overlays/local/observability/dashboards/service-health.json) dashboard at `http://localhost:3000/d/flowspace-service-health`. Select the `service` label of the alert.
2. In Grafana Explore, run this query against the Prometheus data source to find the routes and methods that fail:

   ```promql
   sum by (service, http_route, http_response_status_code) (rate(http_server_request_duration_seconds_count{http_response_status_code=~"5.."}[5m]))
     or sum by (service, rpc_method, rpc_grpc_status_code) (rate(rpc_server_call_duration_seconds_count{rpc_grpc_status_code=~"2|4|13|14"}[5m]))
   ```

   If each route of `workspace-api` returns `503`, Workspace cannot check sessions, so go to the next check. If 1 route or method fails with `500` or `Internal`, read the logs in the fourth check.
3. Inspect the pods of the service and of Identity, because each Workspace request checks its session with Identity:

   ```bash
   kubectl -n flowspace-local get pods -l 'app.kubernetes.io/name in (identity-api, workspace-api, identity-postgres, workspace-postgres)'
   ```

   A pod that is not `Running` and ready, or a high restart count, points to the cause.
4. In Grafana Explore, run this query against the Loki data source to read the failed requests of the service:

   ```logql
   {service="workspace-api"} | json | msg="request_completed" | outcome="failure"
   ```

   Replace `workspace-api` with the `service` label of the alert. The `operation` and `status` fields name the failed call. Follow the `trace_id` of a line to its trace in Tempo.

## Resolution

### Identity is unavailable

1. Run `kubectl -n flowspace-local describe deployment identity-api`. Then make sure that it has 1 desired replica.
2. If the deployment has 0 replicas, run `kubectl -n flowspace-local scale deployment identity-api --replicas=1`.
3. If the pod restarts, run `kubectl -n flowspace-local logs deployment/identity-api --previous`. Then fix the error in the `process_failed` line.
4. If the error is `identity schema unavailable`, the Identity database lost its schema after a restart. Run `tilt trigger identity-migrate`. Then delete the `identity-api` pod so that it starts again.

The Error rate panel shows no errors for the service, and the alert sends a resolved email within 5 minutes.

### Grafana cannot query Prometheus

1. Run `kubectl -n flowspace-local get pods -l app.kubernetes.io/name=prometheus`.
2. If the deployment has 0 replicas, run `kubectl -n flowspace-local scale deployment prometheus-server --replicas=1`.
3. If the pod restarts, run `kubectl -n flowspace-local logs deployment/prometheus-server -c prometheus-server --previous`. Then fix the error.

The rule leaves the error state at its next evaluation, and Mailpit receives a resolved `DatasourceError` email.

### A database is unavailable

1. Run `kubectl -n flowspace-local get pods`. Then find the PostgreSQL pod of the service.
2. If the pod is not ready, run `kubectl -n flowspace-local logs <pod>`. Then fix the error.

The Database connections panel of the Service health dashboard shows connections again, and the Error rate panel shows no errors.

### A code change returns errors

1. Find the latest change to the service with `git log -- services/<service>`.
2. Revert the change on a branch. Then run `tilt up` to deploy it.

The Error rate panel shows no errors after the new pod starts.

## Escalation

If the error rate stays above 5% after these steps, open a GitHub Issue with the `type:fix` label. Attach the result of the second check, the pod list of the third check, and 3 failed log lines with their trace IDs.
