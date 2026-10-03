# Spec: Observability alerts and runbooks

Module ID: `observability-alerts-and-runbooks`

Status: Approved

## Objective

Tell developers who run Flowspace in the `local` cluster when users or event delivery feel a failure, and tell them what to do next. The first users are these developers, and all data is disposable under [ADR-0022](../adr/0022-single-host-storage-holds-disposable-data.md).

The capability must answer three questions without a developer watching the dashboards: does a service return errors or answer slowly now, do events or emails stop moving, and does the host lose telemetry or run out of disk?

Each alert links to a runbook that names the first check and the next action. The thresholds are starting values for a learning environment without traffic history or service level objectives (SLOs). Later work adjusts them from measured data.

## Scope and ADRs

The scope covers eight Grafana alert rules, one contact point, one notification policy, the Mailpit NetworkPolicy change for Grafana, and one runbook for each rule in `docs/runbooks/`.

This capability excludes these items:

- The `staging` and `production` environments and their contact points.
- Paging services, chat notifications, and on-call schedules.
- Alerts on causes such as pod restarts, CPU use, or memory use. Those stay on the dashboards.
- Service level objectives and error budgets.
- Alerts on log or trace queries.
- Alerts on the availability of Grafana, Prometheus, or Mailpit. No component watches the alert pipeline itself.

These decisions apply:

- [ADR-0022](../adr/0022-single-host-storage-holds-disposable-data.md): alert state and notifications are disposable.
- [ADR-0030](../adr/0030-telemetry-is-bounded-and-non-blocking.md): alert labels stay inside the bounded metric labels.
- [ADR-0038](../adr/0038-local-metrics-reach-a-single-host-prometheus-through-alloy.md): alert rules read the metrics in Prometheus.
- [ADR-0039](../adr/0039-local-alerts-run-in-grafana-and-notify-through-mailpit.md): Grafana evaluates the rules from Git, sends email to Mailpit, uses the `page` and `ticket` severities, and links each rule to a runbook.

## Contract

### Alert rules

Each rule reads the metrics that `observability-metrics-and-dashboards` defines.

| Rule title | Severity | Condition | Pending period | Labels |
| --- | --- | --- | --- | --- |
| `API error rate is high` | `page` | More than 5% of the HTTP and gRPC server requests of a service end with an error status, and the service receives at least 0.1 requests each second, over the last 5 minutes | 5 minutes | `environment`, `service` |
| `API latency is high` | `ticket` | The p99 HTTP or gRPC server duration of a service is above 2.5 seconds, and the service receives at least 0.1 requests each second, over the last 5 minutes | 10 minutes | `environment`, `service` |
| `Outbox events are stuck` | `page` | The oldest unpublished Identity outbox event is older than 5 minutes | 5 minutes | `environment` |
| `Email consumer lag is not decreasing` | `ticket` | The email worker consumer lag stays above 0 for 10 minutes and is not lower than 10 minutes earlier | 0 minutes | `environment` |
| `Worker metrics are missing` | `ticket` | Prometheus has no outbox age or consumer lag sample from `identity-worker` in the last 5 minutes | 5 minutes | `environment` |
| `Email delivery fails` | `ticket` | At least one email delivery ended with the `failed` or `commit_failed` outcome in the last 15 minutes | 0 minutes | `environment`, `identity_email_kind` |
| `Telemetry is dropped` | `ticket` | Alloy refuses or fails to send spans, metric points, or log entries in each evaluation | 10 minutes | `environment`, Alloy component name |
| `Host disk is almost full` | `ticket` | A node-local volume reports more than 90% of its capacity in use | 10 minutes | `environment`, `persistentvolumeclaim` |

An error status is an HTTP 5xx status or a gRPC code that `observability-logs-and-traces` treats as an error.

### Alert annotations

| Annotation | Content |
| --- | --- |
| `summary` | One sentence with the rule title and the value that fired it, such as the error percentage |
| `runbook_url` | The GitHub URL of the runbook on `main` |
| `dashboard_url` | The full Grafana URL of the dashboard that shows the symptom, `http://localhost:3000/d/<dashboard UID>`, with a UID from the [dashboards](observability-metrics-and-dashboards.md#dashboards) of `observability-metrics-and-dashboards`, such as `http://localhost:3000/d/flowspace-service-health` |

The Grafana root URL `http://localhost:3000` is the local address of the Grafana port forward.

### Notification emails

| Part | Content |
| --- | --- |
| Recipient | `alerts@flowspace.local` |
| Subject | The status, the severity, and the rule title, such as `[FIRING] page: API error rate is high` |
| Body | The `summary`, the labels, the `runbook_url`, and the `dashboard_url` of each alert in the group |

### Runbooks

Each rule has one runbook in `docs/runbooks/` that follows the [runbook conventions](../conventions/runbooks.md).

| Rule title | Runbook file |
| --- | --- |
| `API error rate is high` | `docs/runbooks/api-error-rate-is-high.md` |
| `API latency is high` | `docs/runbooks/api-latency-is-high.md` |
| `Outbox events are stuck` | `docs/runbooks/outbox-events-are-stuck.md` |
| `Email consumer lag is not decreasing` | `docs/runbooks/email-consumer-lag-is-not-decreasing.md` |
| `Worker metrics are missing` | `docs/runbooks/worker-metrics-are-missing.md` |
| `Email delivery fails` | `docs/runbooks/email-delivery-fails.md` |
| `Telemetry is dropped` | `docs/runbooks/telemetry-is-dropped.md` |
| `Host disk is almost full` | `docs/runbooks/host-disk-is-almost-full.md` |

## Behavior

### Evaluation

- Evaluate all rules in one rule group every minute. A rule fires when its condition stays true for its pending period.
- If the query of a rule returns no data, set the rule state to normal. Service metrics have no series while a service has no traffic.
- `Worker metrics are missing` reports missing worker data instead. The worker reports the outbox age and consumer lag every 30 seconds, so missing samples mean that the worker, Alloy, or Prometheus stopped. This rule keeps `Outbox events are stuck` and `Email consumer lag is not decreasing` from firing when only the data is missing.
- If Grafana cannot query Prometheus, set the rule state to error. Grafana sends a notification for the error through the same contact point.
- The latency threshold of 2.5 seconds is a bucket bound of the latency histograms, so the p99 estimate does not depend on interpolation inside a bucket.
- Each volume reports the host filesystem, so `Host disk is almost full` fires once for each volume when the host disk is almost full.
- The email worker retries a failed delivery without a limit, and each failed attempt counts as `failed`, so one short SMTP failure fires `Email delivery fails`.

### Notification policy

- Group notifications by rule title and `service`, and send one email through Mailpit for each notification group.
- Wait 30 seconds before the first notification of a new group, and 5 minutes before a notification about new alerts in a group that already notified.
- Repeat a notification for a firing `page` group every 4 hours and for a firing `ticket` group every 24 hours.
- Send a resolved email when all alerts in a group stop firing.
- Send both severities to the Mailpit contact point. Keep the `severity` label on each alert so that a later environment can route by it.

### Security and abuse

- NetworkPolicy allows Mailpit port 1025 from Grafana as well as from `identity-worker`.
- Grafana sends email inside the cluster without SMTP authentication or TLS. The contact point has no address outside the `flowspace.local` domain.

### Diagnostics

The capability records the rule states in Grafana and the notifications in Mailpit. Alert labels, annotations, and notification emails must never contain these values:

- User IDs, subject IDs, session IDs, workspace IDs, request IDs, or trace IDs.
- Email addresses other than the contact point address, provider account names, tokens, passwords, or codes.
- Raw URL paths, query strings, or error message text.

The error notification for a failed Prometheus query is an exception. It can contain the error text from Grafana about the connection to Prometheus, because that text has no user data.

When Mailpit is unavailable, `Email delivery fails` can fire, but Grafana cannot deliver that notification or any other one. The rule state in Grafana still shows the alert. Mailpit keeps no messages when its pod restarts, so alert emails from before the restart are lost. A `ticket` email is sent again only after 24 hours, so the current state is in Grafana alerting, not in Mailpit.

## Testing strategy

| Risk | Test level |
| --- | --- |
| A provisioned rule has no runbook, or its `runbook_url` points to a missing file | Unit |
| A rule has a severity, label, or annotation outside the contract | Unit |
| A rule does not fire when its symptom exists for the pending period | Cluster |
| A rule fires while the cluster is idle and healthy | Cluster |
| A firing or resolved alert does not reach Mailpit | Cluster |
| Missing worker metrics do not fire `Worker metrics are missing`, or fire `Outbox events are stuck` | Cluster |
| A notification contains an ID, email address, path, or error text that the contract forbids | Cluster |
| Grafana cannot reach Mailpit, or another pod can reach Mailpit | Cluster |

## Implementation boundaries

Always do these actions:

- Provision rules, the contact point, and the notification policy from files in the `local` overlay.
- Start the `First checks` of the `Worker metrics are missing` runbook with a check that Alloy and Prometheus still receive metrics, and then check the `identity-worker` pod.
- Write each runbook from a real check in the local cluster, and run its first query before the change merges.
- In the change that adds the first runbook, replace the `Examples` section of the [runbook conventions](../conventions/runbooks.md) with a link to that runbook.

Ask the maintainer before these actions:

- Change a threshold, pending period, severity, or repeat interval in this specification.
- Add a rule, a contact point, or a notification channel.
- Change the Mailpit NetworkPolicy beyond the Grafana rule.

Never do these actions:

- Do not add an alert on a cause, such as a pod restart or CPU use.
- Do not send notifications outside the cluster.
- Do not create or edit rules in the Grafana UI as the source.

## Success criteria

1. Given the cluster runs with no faults and normal test traffic for 30 minutes, When a developer opens Grafana alerting and Mailpit, Then no rule fires and no alert email exists.
2. Given Identity is stopped while a client reads workspaces at least once each second, When 5 minutes pass, Then `API error rate is high` fires for `workspace-api`, and Mailpit receives a `page` email with the runbook and dashboard links.
3. Given the fault in criterion 2 ends, When the error rate returns below the threshold, Then Mailpit receives a resolved email for the same group.
4. Given a service answers a route in more than 2.5 seconds for 10 minutes, When the rule evaluates, Then `API latency is high` fires for that service with the `ticket` severity.
5. Given the broker is stopped while signups create outbox events, When the oldest event is older than 5 minutes for 5 minutes, Then `Outbox events are stuck` fires.
6. Given `identity-worker` is stopped, When Prometheus has no outbox age or consumer lag sample for 10 minutes, Then `Worker metrics are missing` fires, and `Outbox events are stuck` and `Email consumer lag is not decreasing` do not fire.
7. Given the email worker cannot consume while events are published, When the lag stays above 0 and does not decrease for 10 minutes, Then `Email consumer lag is not decreasing` fires.
8. Given a test NetworkPolicy blocks `identity-worker` from Mailpit while Grafana can still reach it, When one email delivery fails, Then `Email delivery fails` fires for the kind of that email, and Mailpit receives the alert email.
9. Given Tempo is stopped while services send spans, When Alloy fails to send spans for 10 minutes, Then `Telemetry is dropped` fires.
10. Given a test copy of `Host disk is almost full` has a threshold below the current host disk use, When 10 minutes pass, Then the test rule fires once for each volume. The provisioned rule keeps the 90% threshold.
11. Given Prometheus is stopped, When the rules evaluate, Then Grafana shows the rules in the error state and sends an error notification.
12. Given each rule file in the repository, When the runbook check runs, Then every rule has `severity`, `summary`, `runbook_url`, and `dashboard_url`, and every `runbook_url` names an existing file in `docs/runbooks/`.
13. Given the alert emails from criteria 2 through 9, When a developer reads them, Then no email contains an ID, an email address other than the contact point, a raw path, or error text.
14. Given a pod other than Grafana or `identity-worker`, When it connects to Mailpit port 1025, Then the connection fails.
