# Runbook conventions

This convention defines the file and format of 1 runbook in `docs/runbooks/`.

A runbook tells a developer what an alert means and what to do when it fires. The rule title is the `title` field of the provisioned Grafana alert rule.

The [Observability alerts and runbooks](../specs/observability-alerts-and-runbooks.md) specification lists the alert rules and their runbooks.

## Template

A runbook has a title, an alert line, a severity line, [Meaning](#meaning), [First checks](#first-checks), [Resolution](#resolution), and [Escalation](#escalation).

```markdown
# Runbook: <rule title>

Alert: `<rule title>`

Severity: `<severity label value>`

## Meaning

<symptom and the condition that fires the alert>

## First checks

1. <check that finds the cause>

## Resolution

### <common cause>

1. <step that removes the cause>

<result that shows the fix worked>

## Escalation

<condition that ends the runbook and the next action>
```

### Title

- Write the rule title exactly, such as `API error rate is high`.

### Alert line

- Put the line after the title, separated by a blank line.
- Write the rule title exactly, in backticks.

### Severity line

- Put the line after the alert line, separated by a blank line.
- Write the `severity` label value of the rule, `page` or `ticket`.

### Meaning

- Write 1 paragraph that states the symptom first and then the condition, threshold, and duration of the rule.
- State who or what feels the symptom, such as API clients or the email recipients.
- Link to the rule file in the repository. Do not copy the full rule expression.

### First checks

- Write a numbered list in the order to follow.
- Write each query in a code block with its language, such as `promql` or `logql`.
- Start with the dashboard panel that shows the symptom. Link to the dashboard file and name the panel.
- State what query result points to which cause.
- Before a check uses a host, a namespace, or a port forward, name it and give the command that the check needs.

### Resolution

- Write 1 `###` subsection for each common cause, named after the cause.
- Under each cause, write the steps as a numbered list, and put the result after the list.
- Do not include a step that deletes data unless the step states the data loss and the cause requires it.

### Escalation

- Name a GitHub Issue with the `type:fix` label as the next action, and state the evidence to attach.

## Rules

### Naming and location

- Save each runbook as `docs/runbooks/<alert-slug>.md`. Write the slug as the rule title in lowercase words separated by hyphens.

### Workflow

- Write 1 runbook for each alert rule.
- Add the runbook in the same PR as its rule.

### Links and tracking

- Set the `runbook_url` annotation of the rule to the GitHub URL of the runbook file on `main`.

### Changes

- If a rule changes its condition, threshold, or severity, update the runbook in the same PR.
- If a rule is removed, remove its runbook in the same PR.
- Apply a change of this convention to every existing runbook in the same PR as the change.

## Differences from the observability-and-instrumentation skill

The [Observability and Instrumentation](../../.agents/skills/observability-and-instrumentation/SKILL.md) skill covers runbooks, but this convention applies where the skill and this convention differ:

- Follow the [Template](#template). The skill writes 3 bold labels: "Means", "First check", and "Escalate to".
- Follow the [Escalation](#escalation) rules. The skill escalates to an on-call channel or rotation.

## Examples

These first sentences of a [Meaning](#meaning) section state the symptom before the condition:

```markdown
## Meaning

API clients receive server errors from 1 Flowspace service. The alert fires when, over the last 5 minutes, more than 5% of the HTTP and gRPC server requests of a service end with an error status and the service receives at least 0.1 requests each second. The condition must stay true for 5 minutes.
```
