# Evidence map conventions

An evidence map links a module's approved success criteria to tests and review evidence. It helps a reviewer find the proof for each claimed result.

## Template

Save the map as `docs/evidence/<module-id>.md`:

```markdown
# <Capability name> evidence

This map links the [approved specification](../specs/<module-id>.md#success-criteria) to tests and review evidence.

| Criterion | Evidence | Threat IDs |
| --- | --- | --- |
| 1. <Observable result> | [<Named test>](<test-file>) checks <asserted outcome>. | <Applicable IDs> |
```

Omit the `Threat IDs` column when the specification has no threat IDs. Add `## Open gaps` after the table only when a criterion or applicable threat lacks evidence.

## Rules

- Keep one row per applicable success criterion in the order used by the approved specification. Summarize the result without copying the full criterion.
- Link each row to the test or review artifact that proves its result. Name the behavior the evidence checks. A passing suite without an identified test does not prove a specific behavior.
- Include every applicable threat ID in a row or a short note below the table. Link to the threat model when it defines those IDs.
- If evidence is missing, describe the gap under `Open gaps` and link its follow-up Issue when one exists. Keep the related plan checkpoint open until the gap is resolved.
- Keep dated command results, coverage numbers, and CI logs in the task Issue or PR checks. The map records where to find evidence, not a snapshot of one run.
