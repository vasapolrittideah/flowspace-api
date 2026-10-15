# Runbook: Outbox events are stuck

Alert: `Outbox events are stuck`

Severity: `page`

## Meaning

Identity events do not leave the outbox, so new users do not receive their verification and recovery emails. The alert fires when the oldest unpublished Identity outbox event is older than 5 minutes. The condition must stay true for 5 minutes. The [`deploy/overlays/local/observability/alerting/rules.json`](../../deploy/overlays/local/observability/alerting/rules.json) file holds the query. An email with the `DatasourceError` alert name and this runbook means that Grafana cannot query Prometheus.

## First checks

1. Open Grafana with `kubectl -n flowspace-local port-forward svc/grafana 3000:80`, or use the Grafana port forward of Tilt. Then open the Oldest outbox event age panel of the [Event delivery](../../deploy/overlays/local/observability/dashboards/event-delivery.json) dashboard at `http://localhost:3000/d/flowspace-event-delivery`.
2. In Grafana Explore, run this query against the Loki data source to find the publish failures of the outbox relay:

   ```logql
   {service="identity-worker"} |= "outbox_publish_failed"
   ```

   Lines in the last minutes mean that the relay cannot publish to Redpanda, so go to the next check. No lines mean that the relay does not run, so go to the fourth check.
3. Inspect the Redpanda pod:

   ```bash
   kubectl -n flowspace-local get pods -l app.kubernetes.io/name=redpanda
   ```

   No pod, or a pod that is not ready, points to Redpanda.
4. Inspect the `identity-worker` pod:

   ```bash
   kubectl -n flowspace-local get pods -l app.kubernetes.io/name=identity-worker
   ```

   A pod that is not ready, or a high restart count, points to the worker.

## Resolution

### Redpanda is unavailable

1. Run `kubectl -n flowspace-local describe statefulset redpanda`. Then make sure that it has 1 desired replica.
2. If the statefulset has 0 replicas, run `kubectl -n flowspace-local scale statefulset redpanda --replicas=1`.
3. If the pod restarts, run `kubectl -n flowspace-local logs statefulset/redpanda -c redpanda --previous`. Then fix the error.

The relay publishes the queued events, the Oldest outbox event age panel falls to 0, and the alert sends a resolved email within 10 minutes.

### The relay does not run

1. Follow the worker steps in [`worker-metrics-are-missing.md`](worker-metrics-are-missing.md).
2. If the worker runs and logs no publish failure, open a GitHub Issue as the Escalation section states.

The Oldest outbox event age panel falls to 0.

## Escalation

If the outbox age keeps growing after these steps, open a GitHub Issue with the `type:fix` label. Attach a screenshot of the Oldest outbox event age panel, the pod lists of the third and fourth checks, and 3 lines of the second check.
