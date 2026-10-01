# Module plan conventions

This convention defines the format for a module plan based on an approved specification.

## Template

A plan has the following fields and sections:

```markdown
# Implementation plan: <capability name>

Module id: `<module-id>`

Status: <state of the plan>.

## Overview

<capability and approved specification>

## Architecture decisions

<decisions that set task boundaries>

## Dependency graph

<order between major pieces of work>

## Task list

Tasks are tracked in the [flowspace-api GitHub Project](https://github.com/users/vasapolrittideah/projects/4) under the [<capability name> milestone](<milestone URL>).

<tasks by phase, their checkpoints, and their Issues>

## Risks and controls

<material risks, their impacts, and their controls>
```

### Title

- Copy the capability name from the approved specification.

### Module id

- Copy the module ID from the approved specification, in backticks.

### Status

- Write `Draft` before plan approval, `Approved` after approval, and `Complete` after final verification.

### Overview

- Link the approved specification.

### Architecture decisions

- Treat open architecture proposals as undecided.

### Dependency graph

- Draw the graph as a Mermaid diagram.

### Task list

- Keep the fixed sentence that links the GitHub Project and the plan's milestone.
- Number the tasks by phase. Add a checkpoint with checkable outcomes after each phase.
- Track tasks in GitHub Issues and their status in the repository GitHub Project.
- Keep an ordered index of Issue links and completed checkpoints as the completion record.
- When the module is complete, record the PR with the final verification.
- Do not keep a duplicate task checklist.

### Risks and controls

- Write a table with one row for each material risk.

## Rules

- Save one plan as `tasks/<module-id>.md`. Use the same module ID as its specification in `docs/specs/` and the section order shown in the template.
- Use `tasks/.todo.md` only while preparing Issues. Write each draft with the [Issue template](github-issues.md#template).
- When creating Issues from the drafts, create or reuse the approved plan's [GitHub milestone](github-milestones.md).
- After every Issue appears in the Project with `Todo` status and in the milestone, replace the drafts with an ordered index of Issue links and delete `tasks/.todo.md`.
- Update the plan while creating GitHub Issues and after final verification.

## Examples

The [Identity plan](../../tasks/identity-signup-and-email-verification.md) and [Workspace plan](../../tasks/workspace-creation-and-reading.md) show this format.
