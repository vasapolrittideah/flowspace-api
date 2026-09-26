# Module plan conventions

This convention defines the format for a module plan based on an approved specification.

## Template

A plan has the following fields and sections:

```markdown
# Implementation plan: <capability name>

Module id: `<module-id>`

Status: Draft.

## Overview

<capability and approved specification>

## Architecture decisions

<decisions that set task boundaries>

## Dependency graph

<Mermaid diagram of major work>

## Task list

Tasks are tracked in the [flowspace-api GitHub Project](https://github.com/users/vasapolrittideah/projects/4) under the [<capability name> milestone](<milestone-url>).

<numbered tasks, phases, checkpoints, and Issue links>

## Risks and controls

<table of material risks, impacts, and controls>
```

| Part | Content and format |
| --- | --- |
| Header | The capability name and module ID from the approved specification. The status is `Draft`, `Approved`, or `Complete`. |
| Overview | State the capability and link the approved specification. |
| Architecture decisions | The decisions that set task boundaries. |
| Dependency graph | Show the order between major pieces of work in a Mermaid diagram. |
| Task list | Link the GitHub Project and the plan's milestone, then number tasks by phase with checkable checkpoints and an ordered index of Issue links. |
| Risks and controls | Name each material risk, its impact, and its control in a table. |

## Rules

- Treat open architecture proposals as undecided.
- Use `Draft` before plan approval, `Approved` after approval, and `Complete` after final verification. When the module is complete, record the PR with the final verification.
- Save one plan as `tasks/<module-id>.md`. Use the same module ID as its specification in `docs/specs/` and the section order shown in the template.
- Add a checkpoint with checkable outcomes after each phase. Track tasks in GitHub Issues and their status in the repository GitHub Project. Keep an ordered index of Issue links and completed checkpoints as the completion record. Do not keep a duplicate task checklist.
- Use `tasks/.todo.md` only while preparing Issues. Write each draft with the [Issue template](github-issues.md#template).
- When creating Issues from the drafts, create or reuse the approved plan's [GitHub milestone](github-milestones.md).
- After every Issue appears in the Project with `Todo` status and in the milestone, replace the drafts with an ordered index of Issue links and delete `tasks/.todo.md`.
- Update the plan while creating GitHub Issues and after final verification.

## Examples

The [Identity plan](../../tasks/identity-signup-and-email-verification.md) and [Workspace plan](../../tasks/workspace-create-read.md) show this format.
