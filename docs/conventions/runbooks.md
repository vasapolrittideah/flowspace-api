# Runbook conventions

This convention defines the file and format of one runbook in `docs/runbooks/`. A runbook tells a developer what an alert means and what to do when it fires.

## Template

```markdown
# Runbook: <alert name>

Alert: `<alert rule name>`

Severity: <page or ticket>

## Meaning

<the symptom that users or event delivery feel, and the condition that fires the alert>

## First checks

<numbered checks that find the cause>

## Resolution

<actions that remove the common causes>

## Escalation

<the condition that ends the runbook and the next action>
```

### Alert name

- Write the title as `# Runbook: <alert name>` with the alert title from Grafana, such as "API error rate is high".

### Alert rule name

- Write the rule name exactly as the `title` field of the provisioned Grafana rule.

### Severity

- Write the `severity` label value of the rule, `page` or `ticket`.

### Meaning

- Write one paragraph that states the symptom first and then the condition, threshold, and duration of the rule.
- State who or what feels the symptom, such as API clients or the email recipients.
- Do not copy the full rule expression. Link to the rule file in the repository instead.

### First checks

- Write a numbered list in the order to follow.
- Start with the dashboard panel that shows the symptom. Link to the dashboard file and name the panel.
- Give each query as a code block with its language, such as `promql` or `logql`, and state what result points to which cause.
- Name the host, namespace, or port-forward command that a check needs before the check uses it.

### Resolution

- Write one `###` subsection for each common cause, named after the cause.
- Under each cause, write numbered steps and the result that shows the fix worked.
- Do not include a step that deletes data unless the step states the data loss and the cause requires it.

### Escalation

- State the condition that ends the runbook, such as a cause that the steps do not cover.
- Name the next action, such as a GitHub Issue with the `type:fix` label and the evidence to attach.

## Rules

- Save each runbook as `docs/runbooks/<alert-slug>.md`. The slug is the alert name in lowercase words separated by hyphens.
- Write one runbook for each alert rule. Add the runbook in the same PR as its rule.
- Set the `runbook_url` annotation of the rule to the GitHub URL of the runbook file on `main`.
- When a rule changes its condition, threshold, or severity, update the runbook in the same PR.
- When a rule is removed, remove its runbook in the same PR.

## Examples

This `Meaning` section states the symptom before the condition:

```markdown
## Meaning

API clients receive server errors from one Flowspace service. The alert fires when more than 5% of the requests to a service return HTTP 5xx or a gRPC error status for 5 minutes, and the service receives at least 1 request each second.
```
