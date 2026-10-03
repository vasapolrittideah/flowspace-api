# Runbook conventions

This convention defines the file and format of one runbook in `docs/runbooks/`. A runbook tells a developer what an alert means and what to do when it fires. The rule title is the `title` field of the provisioned Grafana alert rule. The [alerts and runbooks specification](../specs/observability-alerts-and-runbooks.md) lists the alert rules and their runbooks.

## Template

A runbook has a title, an alert line, a severity line, `Meaning`, `First checks`, `Resolution`, and `Escalation`.

```markdown
# Runbook: <rule title>

Alert: `<rule title>`

Severity: <severity label value>

## Meaning

<symptom and the condition that fires the alert>

## First checks

<checks that find the cause>

## Resolution

### <common cause>

<steps that remove the cause>

## Escalation

<condition that ends the runbook and the next action>
```

### Title

- Write the rule title exactly, such as "API error rate is high".

### Alert line

- Put the line after the title, separated by a blank line.
- Write the rule title exactly, in backticks.

### Severity line

- Put the line after the alert line, separated by a blank line.
- Write the `severity` label value of the rule, `page` or `ticket`.

### Meaning

- Write one paragraph that states the symptom first and then the condition, threshold, and duration of the rule.
- State who or what feels the symptom, such as API clients or the email recipients.
- Link to the rule file in the repository. Do not copy the full rule expression.

### First checks

- Write a numbered list in the order to follow.
- Start with the dashboard panel that shows the symptom. Link to the dashboard file and name the panel.
- Write each query in a code block with its language, such as `promql` or `logql`, and state what result points to which cause.
- Before a check uses a host, a namespace, or a port forward, name it and give the command that the check needs.

### Resolution

- Write one `###` subsection for each common cause, named after the cause.
- Under each cause, write numbered steps and the result that shows the fix worked.
- Do not include a step that deletes data unless the step states the data loss and the cause requires it.

### Escalation

- State the condition that ends the runbook, such as a cause that the steps do not cover.
- Name the next action, such as a GitHub Issue with the `type:fix` label and the evidence to attach.

## Rules

### Naming and location

- Save each runbook as `docs/runbooks/<alert-slug>.md`. Write the slug as the rule title in lowercase words separated by hyphens.

### Workflow

- Write one runbook for each alert rule. Add the runbook in the same PR as its rule.

### Links and tracking

- Set the `runbook_url` annotation of the rule to the GitHub URL of the runbook file on `main`.

### Changes

- If a rule changes its condition, threshold, or severity, update the runbook in the same PR.
- If a rule is removed, remove its runbook in the same PR.
- Apply a change of this convention to every existing runbook in the same PR as the change.

## Differences from the observability-and-instrumentation skill

The [`observability-and-instrumentation` skill](../../.agents/skills/observability-and-instrumentation/SKILL.md) gives a short runbook with three labeled lines. This convention applies where the two differ:

- Write the lines and sections of the [template](#template), including a `Resolution` section for the common causes. The skill writes three bold labels: `Means`, `First check`, and `Escalate to`.
- End with a GitHub Issue as the next action, as the [escalation rules](#escalation) state. The skill escalates to an on-call channel or rotation.

## Examples

These first sentences of a `Meaning` section state the symptom before the condition:

```markdown
## Meaning

API clients receive server errors from one Flowspace service. The alert fires when, over the last 5 minutes, more than 5% of the HTTP and gRPC server requests of a service end with an error status and the service receives at least 0.1 requests each second. The condition must stay true for 5 minutes.
```
