# GitHub Issue conventions

This convention defines the title and body of a GitHub Issue for one task.

## Template

An Issue has a title and the body fields shown below.

```markdown
Module: `<module-id>`
Estimated scope: <expected size>.

## Description

<task outcome and scope>

## Acceptance criteria

- [ ] <outcome that can be checked on its own>

## Verification

- [ ] Run `<command>` to check <behavior>.

## Files likely touched

- `<source, test, contract, or configuration path>`
```

| Field | Content and format |
| --- | --- |
| Title | The task outcome. |
| Module | Use the module ID from the approved specification. |
| Estimated scope | State the expected size of the task. |
| Description | State the task outcome and scope. Put prerequisites without an Issue number in a separate paragraph. |
| Acceptance criteria | Independently checkable outcomes, each as a `- [ ]` item. |
| Verification | Commands and the behavior each command checks, each as a `- [ ]` item. |
| Files likely touched | Source, test, contract, and configuration paths. Generated output appears as a folder with a trailing slash and `(generated output)`, such as `gen/go/flowspace/identity/v1/` (generated output). |

## Rules

- Follow the workflow and authority in the [agent instructions](../../AGENTS.md) when creating Issues.
- Use one Issue for each task. Use the body fields in the template order, with the same spelling and capitalization.
- State the task outcome in the Issue title. Keep the title and body consistent with the approved specification, module plan, and task scope.
- Before creating an Issue, compare its title, body, acceptance criteria, verification, file list, and planned blockers with the approved specification and module plan.
- After creating an Issue, apply the [Issue labels](github-labels.md), add it to the repository GitHub Project with `Todo` status, and record its blockers as native GitHub `Blocked by` relationships. Assign the [plan milestone](github-milestones.md) to each Issue in the plan's numbered Task list.
- Use as many acceptance criteria items as the task needs.
- Keep inspection of a command's output in the same Verification item when they form one check. Put separate manual checks in separate items.
- Add one native GitHub `Blocked by` relationship for each blocking Issue. Use these relationships as the dependency list.
- For generated output, list only its folder. Do not list generated file names.

## Final Prove task

The final task in each module plan proves the approved specification and records the evidence. Use the Issue template above.

- Title the Issue `Prove <capability> against its specification`. Name the capability so readers can distinguish it from other modules.
- In Description, name the final test scope and any cross-service evidence owned by another Issue. Link to that evidence instead of repeating its implementation.
- In Acceptance criteria, require an [evidence map](evidence-maps.md) with one row per success criterion and a reference to each applicable threat ID. Each row must link to a named test or review artifact and state what it proves.
- Add acceptance criteria for the module's public, private, failure, and cross-service paths when they apply. Name the outcomes that still need proof.
- If evidence is missing, record the gap and its follow-up Issue in the map. Keep the Prove task and final plan checkpoint open until the gap is resolved.
- In Verification, list `task check:task`, `task markdown:check`, PR diff inspection, and CI review. Add separate command items for the module's applicable integration, smoke, contract, and generation checks. Record command results in the Issue or PR checks, not in the map.
- Add `Blocked by` relationships for actual blocking Issues. Include `docs/evidence/<module-id>.md`, `docs/specs/<module-id>.md`, `tasks/<module-id>.md`, and likely test paths in Files likely touched.
- When the evidence is complete, mark the specification `Implemented` and the plan `Complete`. Record the final PR in the plan.

## Examples

The [Workspace creation Issue](https://github.com/vasapolrittideah/flowspace-api/issues/48) shows a complete Issue using this template.
