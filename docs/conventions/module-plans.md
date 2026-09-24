# Module plan conventions

This convention defines the format for a module plan based on an approved specification.

## Template

A plan has the following fields and sections:

```text
# Implementation plan: <capability name>

Module id: `<module-id>`

Status: Draft.

## Overview

<capability, approved specification, and GitHub Project>

## Architecture decisions

<decisions that set task boundaries>

## Dependency graph

<Mermaid diagram of major work>

## Task list

<numbered tasks, phases, checkpoints, and Issue links>

## Risks and controls

<table of material risks, impacts, and controls>
```

| Part | How to write it |
| --- | --- |
| Header | The capability name and module ID from the approved specification. The status is `Draft`, `Approved`, or `Complete`. |
| Overview | State the capability, link the approved specification, and name the GitHub Project that tracks tasks. |
| Architecture decisions | The decisions that set task boundaries. |
| Dependency graph | Show the order between major pieces of work in a Mermaid diagram. |
| Task list | Numbered tasks grouped into phases, with checkable checkpoints and an ordered index of Issue links. |
| Risks and controls | Name each material risk, its impact, and its control in a table. |

## Rules

- Follow this convention when writing or updating `tasks/<module-id>.md`.
- Save one plan as `tasks/<module-id>.md`. Use the same module ID as its specification in `docs/specs/` and the section order shown in the template.
- Use `Draft` before plan approval, `Approved` after approval, and `Complete` after final verification. When the module is complete, record the PR with the final verification.
- Treat open architecture proposals as undecided.
- Add a checkpoint with checkable outcomes after each phase. Track tasks in GitHub Issues and their status in the repository GitHub Project. Keep an ordered index of Issue links and completed checkpoints as the completion record. Do not keep a duplicate task checklist.
- Update the plan while creating GitHub Issues and after final verification.

Use `tasks/.todo.md` only while preparing Issues. Write each draft with the [Issue template](github-issues.md#template). After every Issue appears in the Project with `Todo` status, replace the drafts with an ordered index of Issue links and delete `tasks/.todo.md`.

## Examples

The [Identity plan](../../tasks/identity-signup-and-email-verification.md) and [Workspace plan](../../tasks/workspace-create-read.md) show this format.
