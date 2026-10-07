# Architecture decision record conventions

This convention defines the file, format, and status of one architecture decision record (ADR) in `docs/adr/`. A record is one ADR. The index is the table of records in the [Architecture Decision Records](../adr/README.md) file, which also states the purpose of a record.

## Template

A record has a title, a date line, a status line, [Context](#context), [Decision](#decision), [Alternatives Considered](#alternatives-considered), [Consequences](#consequences), and [Sources](#sources).

```markdown
# ADR-<NNNN>: <decision that the record accepts>

Date: <date when the record was written>

Status: <current state of the decision>

## Context

<facts and limits that exist before the decision>

## Decision

<what the project chose and the rules that follow from it>

## Alternatives Considered

### <option that the project rejected>

- Pros: <benefits of the option>
- Cons: <costs of the option>
- Rejected: <reason that decided against the option>

## Consequences

- <effect of the decision>

## Sources

<external documents that the decision depends on, if any>
```

### Title

- Write the record number with four digits.
- Write the decision as a sentence in the present tense, such as "Relational data uses explicit SQL".
- Start the sentence with the thing that the decision controls, not with a verb.
- Do not write only a topic, such as "Database choice".

### Date line

- Put the line directly after the title, separated by a blank line.
- Write the date in `YYYY-MM-DD` format.
- Do not change the date when the status changes.

### Status line

- Put the line after the date line, separated by a blank line.
- Write one value from [Status values](#status-values).
- Link each `ADR-<NNNN>` in the value to the file of that record.
- If a record makes scoped exceptions to several records, write `scoped exceptions to`, and join the records with `and`.

### Context

- Write paragraphs in this order: the problem or need, the current state and its limits, and the questions that earlier records left open. Omit a part that does not apply.
- Link to the earlier records and architecture sections that the decision depends on.
- Do not describe the chosen solution or the alternatives.

### Decision

- In the first paragraph, state the decision in one or two sentences, such as "Use PostgreSQL for primary relational data."
- If the decision applies only to part of the system, such as one environment, state that scope in the first sentence.
- Write the rules that follow from the decision in paragraphs, one topic per paragraph, as the [Format and content](markdown-and-english-prose.md#format-and-content) rules require.
- Use a bullet list only for parallel items of the same kind, such as each service and the data that it owns.
- If the decision fixes a value, such as a limit, a duration, or a retention period, state the exact value.
- Do not add `###` subsections. If the topics of a decision need their own subsections, write a separate record for each topic.

### Alternatives Considered

- Write at least one alternative.
- Name the option in its `###` heading as a noun phrase.
- Under each heading, write three bullets in this order: `Pros:`, `Cons:`, and `Rejected:`.
- Write one sentence after each label.
- Start the sentence with a lowercase letter unless it starts with a name or code.
- In `Rejected:`, compare the option with the chosen decision.
- Do not repeat the `Cons:` text in `Rejected:`.

### Consequences

- Write one bullet for each effect.
- Put open work last, such as a decision that a later record must make.
- Include costs, risks, and the effects of failures, as well as benefits.

### Sources

- Write one bullet for each external document.
- Use the document title as the link text.
- Do not list repository documents. Link to them in [Context](#context) or [Decision](#decision).

## Rules

### Naming and location

- Save the record as `docs/adr/<NNNN>-<slug>.md`. Write the slug as a short form of the title in lowercase words separated by hyphens.
- Give each record the next four-digit number. Do not reuse or skip a number.

### Format and content

- Use the sections through [Consequences](#consequences) in the template order. Add [Sources](#sources) only when the decision depends on external documents.
- Do not add other top-level sections. Add a new section to this convention before you use it in a record.

### Workflow

- Write one record for each decision that is expensive to reverse. Put replaceable tools and libraries in the [Technology stack](../technology-stack.md).

### Links and tracking

- Add a row for each record to the index. Copy the title sentence to the `Decision` column and the value from [Status values](#status-values) to the `Status` column.
- In the index, use only the record number as the link text, such as `0031` instead of `ADR-0031`.

### Status and approval

- Open the PR of a record with the `Accepted` status. The maintainer accepts the record by merging the PR.
- Keep an undecided proposal in the [Open proposals](../architecture.md#open-proposals) section of the architecture until a record accepts it.

### Changes

- If a decision changes, write a new record. In the same PR, change the status line of the earlier record and its row in the index. If the new record only makes a scoped exception to the earlier record, keep the status of the earlier record.
- Apply a change of this convention to new records only. Do not convert accepted records to the new format.
- Do not change the title, date, or sections of an accepted record.

## Differences from the documentation-and-adrs skill

The [`documentation-and-adrs` skill](../../.agents/skills/documentation-and-adrs/SKILL.md) gives a generic ADR template and life cycle. This convention applies where the two differ:

- Save each record in `docs/adr/` with a four-digit number, as the [Naming and location](#naming-and-location) rules state. The skill saves records in `docs/decisions/` with three-digit numbers, such as `ADR-001`.
- Write the date and the status as lines after the title, as the [Template](#template) shows. The skill writes them as `## Status` and `## Date` sections, with the status first.
- Open each record with the `Accepted` status, and keep undecided proposals in the architecture, as the [Status and approval](#status-and-approval) rules state. The skill starts a record with the `Proposed` status.
- Use only the [Status values](#status-values) of this convention. The skill also uses `Deprecated`.

## Reference

### Status values

| Value | Use when |
| --- | --- |
| `Accepted` | The decision applies in full. |
| `Accepted; scoped exception to ADR-<NNNN>` | The decision makes an exception to an earlier accepted record for a named scope. |
| `Accepted, except <part> superseded by ADR-<NNNN>` | A later record replaces part of the decision. |
| `Superseded by ADR-<NNNN>` | A later record replaces the whole decision. |
