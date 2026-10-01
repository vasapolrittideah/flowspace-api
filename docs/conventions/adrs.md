# Architecture decision record conventions

This convention defines the file, format, and status of one architecture decision record (ADR) in `docs/adr/`.

## Template

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

- <effect of the decision, including a cost or open work>

## Sources

<external documents that the decision depends on, if any>
```

### Title

- Write the record number with four digits.
- Write the decision as a sentence in the present tense, such as "Relational data uses explicit SQL".
- Start the sentence with the thing that the decision controls, not with a verb.
- Do not write only a topic, such as "Database choice".

### Date

- Put the line directly after the title, separated by a blank line.
- Write the date in `YYYY-MM-DD` format.
- Do not change the date when the status changes.

### Status

- Put the line after `Date`, separated by a blank line.
- Write one [status value](#status-values).
- Link each `ADR-<NNNN>` in the value to the file of that record.
- Write the same value in the `Status` column of the index.
- In the index, use only the record number as the link text, such as `0031` instead of `ADR-0031`.

### Context

- Write paragraphs in this order: the problem or need, the current state and its limits, and the questions that earlier records left open. Omit a part that does not apply.
- Link to the earlier records and architecture sections that the decision depends on.
- Do not describe the chosen solution or the alternatives.

### Decision

- In the first paragraph, state the decision in one or two sentences, such as "Use PostgreSQL for primary relational data."
- If the decision applies only to part of the system, such as one environment, state that scope in the first sentence.
- Write the rules that follow from the decision in paragraphs, one topic per paragraph, as the [Markdown rules](markdown-and-prose.md#markdown) require.
- Use a bullet list only for parallel items of the same kind, such as each service and the data that it owns.
- State exact values when the decision fixes them, such as limits, durations, and retention periods.
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
- Include costs, risks, and the effects of failures, as well as benefits.
- Put open work last, such as a decision that a later record must make.

### Sources

- Write one bullet for each document.
- Use the document title as the link text.
- Put links to repository documents in `Context` or `Decision`.

## Rules

### Files and numbering

- Write a record for one decision that is expensive to reverse. Put replaceable tools and libraries in the [technology stack](../technology-stack.md).
- Give each record the next four-digit number. Do not reuse or skip a number.
- Save the record as `docs/adr/<NNNN>-<slug>.md`. The slug is a short form of the title in lowercase words separated by hyphens.
- Add a row for each record to the [ADR index](../adr/README.md). Copy the title sentence to the `Decision` column and the [status value](#status) to the `Status` column.

### Sections

- Use the sections through `Consequences` in the template order. Add `Sources` only when the decision depends on external documents.
- Do not add other top-level sections. Add a new section to this convention before you use it in a record.

### Status and changes

- A record merges only with an `Accepted` status. The maintainer accepts a record by merging its PR. Keep an undecided proposal in the [open proposals](../architecture.md#open-proposals) of the architecture until a record accepts it.
- Do not change the title, date, or sections of an accepted record. When a decision changes, write a new record. In the same PR, change the `Status` line of the earlier record and its row in the index.
- Records accepted before this convention keep their titles and sections. A PR that applies this format to them can change only the `Date` and `Status` lines.

## Reference

### Status values

| Value | Use when |
| --- | --- |
| `Accepted` | The decision applies in full. |
| `Accepted; scoped exception to ADR-<NNNN>` | The decision makes an exception to an earlier accepted record for a named scope. The earlier record keeps its status. For several records, write `scoped exceptions to` and join the records with `and`. |
| `Accepted, except <part> superseded by ADR-<NNNN>` | A later record replaces part of the decision. |
| `Superseded by ADR-<NNNN>` | A later record replaces the whole decision. |
