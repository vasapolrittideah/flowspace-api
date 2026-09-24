# Module plan conventions

## Overview

A module plan breaks an approved specification into implementation tasks. It records dependencies, checkpoints, risks, and task links.

## When to Follow

Follow this convention after a specification is approved. Update the plan while creating GitHub Issues and after final verification.

## Template

Save one plan as `tasks/<module-id>.md`. Use the same module ID as its specification in `docs/specs/`. Use these sections in order:

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

### Header

Use the capability name and module ID from the approved specification. Use `Draft` before plan approval, `Approved` after approval, and `Complete` after final verification. When the module is complete, record the PR with the final verification.

### Overview section

State the capability, link the approved specification, and name the GitHub Project that tracks tasks.

### Architecture decisions

Record the decisions that set task boundaries. Treat open proposals as undecided.

### Dependency graph

Show the order between major pieces of work in a Mermaid diagram.

### Task list

Group numbered tasks into phases. After each phase, add a checkpoint with outcomes that can be checked. Track tasks in GitHub Issues and their status in the repository GitHub Project.

Use `tasks/.todo.md` only while preparing GitHub Issues. Write each draft with the [Issue fields](github-issues.md#body-fields). After the Issues are in the GitHub Project with `Todo` status, replace the drafts with an ordered index of Issue links. Do not keep a duplicate task checklist in the plan. Delete `tasks/.todo.md` after every task appears in the Project.

Keep the task links and completed checkpoints as the completion record.

### Risks and controls

Name each material risk, its impact, and its control in a table.

## Examples

The [Identity plan](../../tasks/identity-signup-and-email-verification.md) and [Workspace plan](../../tasks/workspace-create-read.md) show this format.
