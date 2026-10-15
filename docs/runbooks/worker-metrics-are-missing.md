# Runbook: Worker metrics are missing

Alert: `Worker metrics are missing`

Severity: `ticket`

## Meaning

Developers cannot see whether Identity events and emails move, because the outbox age and consumer lag of `identity-worker` are missing. The worker reports both values every 30 seconds, and a failed measurement also leaves a value missing, such as the consumer lag while Redpanda is unavailable. The alert fires when Prometheus has no outbox age or consumer lag sample from `identity-worker` in the last 5 minutes, and the condition must stay true for 5 minutes. While a value is missing, the alert that reads it cannot fire: `Outbox events are stuck` reads the outbox age, and `Email consumer lag is not decreasing` reads the consumer lag. The [`deploy/overlays/local/observability/alerting/rules.json`](../../deploy/overlays/local/observability/alerting/rules.json) file holds the query. An email with the `DatasourceError` alert name and this runbook means that Grafana cannot query Prometheus.

## First checks

1. Open Grafana with `kubectl -n flowspace-local port-forward svc/grafana 3000:80`, or use the Grafana port forward of Tilt. Then open the Oldest outbox event age and Consumer lag panels of the [Event delivery](../../deploy/overlays/local/observability/dashboards/event-delivery.json) dashboard at `http://localhost:3000/d/flowspace-event-delivery`. A gap in a panel shows the time without samples.
2. In Grafana Explore, run this query against the Prometheus data source to make sure that Alloy and Prometheus still receive metrics:

   ```promql
   count by (service) ({service=~".+"})
   ```

   If `identity-api` and `workspace-api` have series and `identity-worker` has none, the worker stopped reporting, so go to the fourth check. If no service has series, Alloy or Prometheus stopped, so go to the next check.
3. Open the Alloy telemetry loss panel of the [Telemetry stack](../../deploy/overlays/local/observability/dashboards/telemetry-stack.json) dashboard at `http://localhost:3000/d/flowspace-telemetry-stack`. Then run this command to inspect the pods of the telemetry stack:

   ```bash
   kubectl -n flowspace-local get pods -l 'app.kubernetes.io/name in (alloy, prometheus)'
   ```

   Refused or failed metric points, or a pod that is not ready, point to Alloy or Prometheus.
4. Inspect the `identity-worker` pod:

   ```bash
   kubectl -n flowspace-local get pods -l app.kubernetes.io/name=identity-worker
   ```

   No pod, a pod that is not ready, or a high restart count points to the worker.
5. In Grafana Explore, run this query against the Loki data source to read the failed measurements of the worker:

   ```logql
   {service="identity-worker"} |~ "identity_outbox_age_unavailable|identity_broker_lag_unavailable|process_failed"
   ```

   `identity_outbox_age_unavailable` points to the Identity database. `identity_broker_lag_unavailable` points to Redpanda.

## Resolution

### The worker does not run

1. Run `kubectl -n flowspace-local describe deployment identity-worker`. Then make sure that it has 1 desired replica.
2. If the deployment has 0 replicas, run `kubectl -n flowspace-local scale deployment identity-worker --replicas=1`.
3. If the pod restarts, run `kubectl -n flowspace-local logs deployment/identity-worker --previous`. Then fix the error in the `process_failed` line.
4. If the error is `identity schema unavailable`, the Identity database lost its schema after a restart. Run `tilt trigger identity-migrate`. Then delete the `identity-worker` pod so that it starts again.

The Oldest outbox event age and Consumer lag panels of the Event delivery dashboard show values again, and the alert sends a resolved email within 10 minutes.

### Alloy or Prometheus does not receive metrics

1. If the Alloy pod is not ready, run `kubectl -n flowspace-local logs deployment/alloy`. Then fix the error.
2. If the Prometheus deployment has 0 replicas, run `kubectl -n flowspace-local scale deployment prometheus-server --replicas=1`.
3. If the Prometheus pod restarts, run `kubectl -n flowspace-local logs deployment/prometheus-server -c prometheus-server --previous`. Then fix the error.

The query of the second check returns series for each service again.

### A measurement fails

1. For `identity_outbox_age_unavailable`, follow the Identity database steps in [`api-error-rate-is-high.md`](api-error-rate-is-high.md).
2. For `identity_broker_lag_unavailable`, run `kubectl -n flowspace-local get pods -l app.kubernetes.io/name=redpanda`. Then make sure that the Redpanda pod is ready.

The log lines of the fifth check stop, and the panels of the Event delivery dashboard show values again.

## Escalation

If the worker metrics stay missing after these steps, open a GitHub Issue with the `type:fix` label. Attach the result of the second check, the pod list of the fourth check, and 3 log lines of the fifth check.
