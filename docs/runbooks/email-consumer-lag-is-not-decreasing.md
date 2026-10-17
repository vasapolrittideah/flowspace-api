# Runbook: Email consumer lag is not decreasing

Alert: `Email consumer lag is not decreasing`

Severity: `ticket`

## Meaning

Identity emails wait in Redpanda, so users receive their verification and recovery emails late or not at all. The alert fires when the consumer lag of the email worker stays above 0 for 10 minutes and is not lower than 10 minutes earlier. It has no pending period. The worker reports the lag itself, so a stopped worker reports no lag and fires `Worker metrics are missing` instead. After the worker misses lag samples, such as after a restart, the alert fires only when Prometheus has at least 18 lag samples from the last 10 minutes and a lag sample above 0 from the minute before that. The [`deploy/overlays/local/observability/alerting/rules.json`](../../deploy/overlays/local/observability/alerting/rules.json) file holds the query. An email with the `DatasourceError` alert name and this runbook means that Grafana cannot query Prometheus.

## First checks

1. Open Grafana with `kubectl -n flowspace-local port-forward svc/grafana 3000:80`, or use the Grafana port forward of Tilt. Then open the Consumer lag and Email deliveries panels of the [Event delivery](../../deploy/overlays/local/observability/dashboards/event-delivery.json) dashboard at `http://localhost:3000/d/flowspace-event-delivery`.
2. In Grafana Explore, run this query against the Prometheus data source to find the delivery results of the last 15 minutes:

   ```promql
   sum by (identity_email_kind, outcome) (increase(identity_email_deliveries_total[15m]))
   ```

   A `failed` or `commit_failed` outcome means that the worker consumes but cannot finish a delivery, so follow the `Email delivery fails` runbook in [`email-delivery-fails.md`](email-delivery-fails.md). No outcome at all means that the worker does not consume, so go to the next check.
3. Inspect the `identity-worker` pod and the Redpanda pod:

   ```bash
   kubectl -n flowspace-local get pods -l 'app.kubernetes.io/name in (identity-worker, redpanda)'
   ```

   A pod that is not ready, or a high restart count, points to the cause.

## Resolution

### Deliveries fail

1. Follow the [`email-delivery-fails.md`](email-delivery-fails.md) runbook.

The Consumer lag panel falls to 0 after the worker delivers the waiting emails, and the alert sends a resolved email.

### The worker does not consume

1. Run `kubectl -n flowspace-local logs deployment/identity-worker`. Then find the error lines.
2. Run `kubectl -n flowspace-local rollout restart deployment identity-worker`.

The Email deliveries panel shows the `delivered` outcome again, and the Consumer lag panel falls to 0.

### Redpanda is unavailable

1. Follow the Redpanda steps in [`outbox-events-are-stuck.md`](outbox-events-are-stuck.md).

The Consumer lag panel falls to 0.

## Escalation

If the lag stays above 0 after these steps, open a GitHub Issue with the `type:fix` label. Attach a screenshot of the Consumer lag panel, the result of the second check, and the pod list of the third check.
