# ADR-0039: Local alerts run in Grafana and notify through Mailpit

Date: 2026-10-02

Status: Accepted

## Context

Developers see failures in the dashboards only while they look at them. The project needs alerts that report a symptom, such as a high error rate or a stuck outbox, when nobody watches the dashboards.

[ADR-0038](0038-local-metrics-reach-a-single-host-prometheus-through-alloy.md) stores local metrics in Prometheus, and [ADR-0037](0037-local-logs-and-traces-use-disposable-single-host-stores.md) runs Grafana without a persistent volume. Mailpit already captures Identity email in the `local` cluster, and only `identity-worker` can connect to its SMTP port. The `local` environment has no on-call person, no paging service, and no budget for one.

ADR-0037 and ADR-0038 leave alert routing open, and the [Open proposals](../architecture.md#open-proposals) section of the architecture lists it.

## Decision

In the `local` environment, Grafana evaluates the alert rules against Prometheus and sends each notification as an email to Mailpit.

Git provisions the alert rules, the contact point, and the notification policy. Grafana has no persistent volume, so silences and the alert state that Grafana keeps reset when the Grafana pod restarts. After a restart, the rules evaluate again from the current metrics.

Each rule has a `severity` label with one of two values. A `page` alert means a user-facing failure that needs action now. A `ticket` alert means a degradation that needs action this week. In `local`, the notification policy sends both values to the same Mailpit contact point. A later environment can route the values to different contact points without changing the rules.

Each rule reports a symptom that users or event delivery feel, such as errors, latency, or delay. A cause, such as a pod restart or high CPU use, stays on a dashboard and has no rule.

Each rule has a `runbook_url` annotation that links to one runbook in `docs/runbooks/` in the repository. A rule without a runbook does not merge.

Grafana connects to Mailpit over SMTP on port 1025 without authentication or TLS, because both run inside the cluster. NetworkPolicy allows that port from Grafana as well as from `identity-worker`. Notifications contain the rule name, severity, summary, and runbook link, and no label values that the metrics contract forbids.

## Alternatives Considered

### Prometheus rules with Alertmanager

- Pros: rules live next to the metrics, and Alertmanager keeps silences in its own storage.
- Cons: it adds a component with its own configuration, storage, and NetworkPolicy to the constrained host.
- Rejected: Grafana already reads Prometheus and can evaluate the same rules without a new component.

### Chat webhook notifications

- Pros: developers see alerts on a phone without opening Mailpit.
- Cons: the webhook URL is a secret, and alert content leaves the self-hosted environment.
- Rejected: Mailpit keeps notifications inside the cluster and needs no new secret.

### One severity

- Pros: simpler rules and one route.
- Cons: a later environment cannot page for user-facing failures without editing every rule.
- Rejected: two labels cost nothing now and keep the routing change outside the rules.

## Consequences

- Developers must open Mailpit through `kubectl port-forward` to read notifications. Nobody receives an alert while the cluster is stopped.
- A Grafana restart clears silences and can send a notification again for an alert that still fires.
- If Grafana, Prometheus, or Mailpit is unavailable, alerts are not evaluated or delivered, and no other component reports that.
- Each new rule needs a runbook, so the runbook format needs its own convention.
- Staging and production need a decision on contact points and paging before their overlays are added.
