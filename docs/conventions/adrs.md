# Architecture decision record conventions

This convention defines the file, format, and status of 1 architecture decision record (ADR) in `docs/adr/`.

The [Architecture Decision Records](../adr/README.md) file states the purpose of a record.

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

- <external document that the decision depends on, if any>
```

### Title

- Write the record number with 4 digits.
- Write the decision as a sentence in the present tense, such as "Relational data uses explicit SQL".
- Start the sentence with the thing that the decision controls, not with a verb.
- Do not write only a topic, such as "Database choice".

### Date line

- Put the line directly after the title, separated by a blank line.
- Write the date in `YYYY-MM-DD` format.
- Do not change the date when the status changes.

### Status line

- Put the line after the date line, separated by a blank line.
- If a record makes scoped exceptions to several records, write `scoped exceptions to`, and join the records with `and`.
- Write 1 value from [Status values](#status-values).
- Link each `ADR-<NNNN>` in the value to the file of that record.

### Context

- Write paragraphs in this order, and omit a part that does not apply:
  1. The problem or need.
  2. The current state and its limits.
  3. The questions that earlier records left open.
- Link to the earlier records and architecture sections that the decision depends on.
- Do not describe the chosen solution or the alternatives.

### Decision

- In the first paragraph, state the decision in 1 or 2 sentences, such as "Use PostgreSQL for primary relational data."
- If the decision applies only to part of the system, such as 1 environment, state that scope in the first sentence.
- Write the rules that follow from the decision in paragraphs, as the [Format and content](markdown-and-english-prose.md#format-and-content) rules require.
- Use a bullet list only for parallel items of the same kind, such as each service and the data that it owns.
- If the decision fixes a value, such as a limit, a duration, or a retention period, state the exact value.
- Do not add `###` subsections. If the topics of a decision need their own subsections, write a separate record for each topic.

### Alternatives Considered

- Name the option in its `###` heading as a noun phrase.
- Under each heading, write 3 bullets in this order:
  1. `Pros:`
  2. `Cons:`
  3. `Rejected:`
- Write 1 sentence after each label.
- Start the sentence with a lowercase letter unless it starts with a name or code.
- In `Rejected:`, compare the option with the chosen decision.
- Write at least 1 alternative.
- Do not repeat the `Cons:` text in `Rejected:`.

### Consequences

- Write 1 bullet for each effect.
- Put open work last, such as a decision that a later record must make.
- Include costs, risks, and the effects of failures, as well as benefits.

### Sources

- Add this section only when the decision depends on external documents.
- Write 1 bullet for each external document.
- Link each document as the [Links and tracking](markdown-and-english-prose.md#links-and-tracking) rules state.
- Do not list repository documents. Link to them in [Context](#context) or [Decision](#decision).

## Rules

### Naming and location

- Save the record as `docs/adr/<NNNN>-<slug>.md`. Write the slug as a short form of the title in lowercase words separated by hyphens.
- Give each record the next 4-digit number. Do not reuse or skip a number.

### Format and content

- Use the sections in the template order.
- Do not add other top-level sections. Add a new section to this convention before you use it in a record.

### Workflow

- Write 1 record for each decision that is expensive to reverse. Put replaceable tools and libraries in the [Technology stack](../technology-stack.md).

### Links and tracking

- Add a row for each record to the index. Copy the title sentence to the `Decision` column and the value from [Status values](#status-values) to the `Status` column.
- In the index, use only the record number as the link text, such as `0031` instead of `ADR-0031`.

### Status and approval

- Open the PR of a record with the `Accepted` status. The maintainer accepts the record by merging the PR.
- Keep an undecided proposal in the [Open proposals](../architecture.md#open-proposals) section of the architecture until a record accepts it.

### Changes

- If a decision changes, follow these steps:
  1. Write a new record.
  2. In the same PR, change the status line of the earlier record and its row in the index. If the new record only makes a scoped exception to the earlier record, keep the status of the earlier record.
- Apply a change of this convention only to new records and to records that a later PR changes. If a later PR changes only the status line of an accepted record, keep the rest of the record in its format.
- Do not change the title, date, or sections of an accepted record.

## Differences from the documentation-and-adrs skill

The [Documentation and ADRs](../../.agents/skills/documentation-and-adrs/SKILL.md) skill gives a generic ADR template and life cycle, but this convention applies where the skill and this convention differ:

- Follow the [Naming and location](#naming-and-location) rules. The skill saves records in `docs/decisions/` with 3-digit numbers, such as `ADR-001`.
- Follow the [Date line](#date-line) and [Status line](#status-line) rules. The skill writes them as `## Status` and `## Date` sections, with the status first.
- Follow the [Status and approval](#status-and-approval) rules. The skill starts a record with the `Proposed` status.
- Follow the [Status line](#status-line) rules. The skill also uses `Deprecated`.

## Reference

### Status values

| Value | Use when |
| --- | --- |
| `Accepted` | The decision applies in full. |
| `Accepted; scoped exception to ADR-<NNNN>` | The decision makes an exception to an earlier accepted record for a named scope. |
| `Accepted, except <part> superseded by ADR-<NNNN>` | A later record replaces part of the decision. |
| `Superseded by ADR-<NNNN>` | A later record replaces the whole decision. |
