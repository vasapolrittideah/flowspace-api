# Runbook: Email delivery fails

Alert: `Email delivery fails`

Severity: `ticket`

## Meaning

Users do not receive 1 kind of Identity email, such as the verification or recovery email. The alert fires when at least 1 email delivery of a kind ended with the `failed` or `commit_failed` outcome in the last 15 minutes. It has no pending period. The worker retries a failed delivery without a limit, so 1 short SMTP failure fires the alert. The [`deploy/overlays/local/observability/alerting/rules.json`](../../deploy/overlays/local/observability/alerting/rules.json) file holds the query. An email with the `DatasourceError` alert name and this runbook means that Grafana cannot query Prometheus.

## First checks

1. Open Grafana with `kubectl -n flowspace-local port-forward svc/grafana 3000:80`, or use the Grafana port forward of Tilt. Then open the Email deliveries panel of the [Event delivery](../../deploy/overlays/local/observability/dashboards/event-delivery.json) dashboard at `http://localhost:3000/d/flowspace-event-delivery`.
2. In Grafana Explore, run this query against the Loki data source to read the failed deliveries:

   ```logql
   {service="identity-worker"} |~ "email_delivery_failed|email_delivery_commit_failed"
   ```

   `email_delivery_failed` means that the worker cannot send the email through Mailpit, so go to the next check. `email_delivery_commit_failed` means that the worker sent the email but cannot commit its offset to Redpanda, so go to the fourth check.
3. Inspect the Mailpit pod and the NetworkPolicies that select `identity-worker` or Mailpit:

   ```bash
   kubectl -n flowspace-local get pods -l app.kubernetes.io/name=mailpit
   kubectl -n flowspace-local get networkpolicy
   ```

   A Mailpit pod that is not ready points to Mailpit. A NetworkPolicy other than `mailpit` and `identity-worker` that selects either pod points to a blocked connection.
4. Inspect the Redpanda pod:

   ```bash
   kubectl -n flowspace-local get pods -l app.kubernetes.io/name=redpanda
   ```

   No pod, or a pod that is not ready, points to Redpanda.

## Resolution

### Mailpit is unavailable

1. If the Mailpit deployment has 0 replicas, run `kubectl -n flowspace-local scale deployment mailpit --replicas=1`.
2. If the pod restarts, run `kubectl -n flowspace-local logs deployment/mailpit --previous`. Then fix the error.

The worker delivers the waiting emails, and the Email deliveries panel shows the `delivered` outcome again. The alert sends a resolved email 15 minutes after the last failure.

### A NetworkPolicy blocks the connection

1. Read the policy with `kubectl -n flowspace-local get networkpolicy <name> -o yaml`.
2. If the policy is a test policy, run `kubectl -n flowspace-local delete networkpolicy <name>`. If it comes from `deploy/`, fix it on a branch.

The worker delivers the waiting emails, and the Email deliveries panel shows the `delivered` outcome again.

### Redpanda is unavailable

1. Follow the Redpanda steps in [`outbox-events-are-stuck.md`](outbox-events-are-stuck.md).

The `email_delivery_commit_failed` lines stop.

## Escalation

If deliveries still fail after these steps, open a GitHub Issue with the `type:fix` label. Attach a screenshot of the Email deliveries panel, 3 lines of the second check, and the pod and policy lists of the third check.
