# Module plan conventions

This convention defines the file, the format, and the status of 1 module plan in `tasks/`.

The [Module specification](module-specs.md) convention defines a module specification.

A module plan breaks an approved module specification into tasks. Each task becomes 1 GitHub Issue, as the [GitHub Issue](github-issues.md) convention defines.

A phase is a group of tasks. A checkpoint lists the outcomes that a reviewer checks after the tasks of a phase are done.

## Template

A plan has a title, a module ID, a status line, [Overview](#overview), [Architecture decisions](#architecture-decisions), [Dependency graph](#dependency-graph), [Task list](#task-list), and [Risks and controls](#risks-and-controls).

````markdown
# Implementation plan: <capability name>

Module ID: `<module ID>`

Status: <current state of the plan>

## Overview

<outcome of the module and its approved specification>

## Architecture decisions

- <decision that sets a task boundary>

## Dependency graph

```mermaid
flowchart TD
    <node ID>[<work>] --> <node ID>[<work>]
```

## Task list

<tracking sentence, if any>

### Phase <number>: <phase name>

- Task <number>: <task title or Issue link>

### Checkpoint: <phase name>

- [ ] <outcome that a reviewer checks>

## Risks and controls

| Risk | Impact | Control |
| --- | --- | --- |
| <risk> | <impact> | <control> |
````

### Title

- Copy the capability name from the title of the approved specification, without the `Spec:` prefix.

### Module ID

- Copy the module ID from the approved specification, in backticks.

### Status line

- Write `Draft`, `Approved`, or `Complete`, without a final period, as the [Status and approval](#status-and-approval) rules state.

### Overview

- Write the overview as paragraphs. Start with the outcome of the module.
- Include the sentence "The plan follows the approved \<capability name\> specification.", and link the capability name to the specification file.

### Architecture decisions

- Write 1 bullet for each decision that sets a task boundary, such as a shared package, a dependency, or a migration.
- Link the ADR that a decision applies, if any.
- Treat an open architecture proposal as undecided. Do not write a decision that depends on one.

### Dependency graph

- Draw the graph as a Mermaid `flowchart TD` diagram in a `mermaid` code block.
- Write an edge `A --> B` when the work of `B` cannot start before the work of `A` is done.
- Give each node a 1-word ID in PascalCase and a label that names its work, such as `Contract[Public recovery contract]`.
- End the graph at 1 node for the final Prove task.
- Do not put task numbers in node labels. The graph can show work that is not a task, such as an approved decision, and native `Blocked by` relationships record the task dependencies.

### Task list

- After the plan leaves the `Draft` status, write the tracking sentence "Tasks are tracked in the flowspace-api GitHub Project under the \<capability name\> milestone." Link "flowspace-api" to the [flowspace-api](https://github.com/users/vasapolrittideah/projects/4) GitHub Project, and link the capability name to the milestone of the plan. While the plan is `Draft`, omit the sentence, because the milestone does not exist yet.
- Write each phase heading as `### Phase <N>: <name>`. Name each phase with a noun phrase in sentence case, such as "Export and storage".
- While the plan is `Draft`, write each task as `- Task <N>: <task title>`. After its Issue exists, replace the task title with a link to the Issue, and use `#<number> <Issue title>` as the link text.
- After the tasks of each phase, write `### Checkpoint: <phase name>` with 1 `- [ ]` item for each outcome. Name the checkpoint of the last phase `### Checkpoint: Complete`.
- Put the final Prove task alone in the last phase.
- Do not keep a task checklist apart from the task lines, because the Issues and the GitHub Project hold the status of each task.

### Risks and controls

- Write the risks as the [Risks table](#risks-table).

#### Risks table

- Write 1 row for each material risk, as the [Module specification](module-specs.md) convention defines a material risk.
- Write the columns in this order.

| Column | How to write |
| --- | --- |
| `Risk` | The failure that can happen, as 1 sentence. |
| `Impact` | What users or the system lose when the failure happens. |
| `Control` | The step in the plan that prevents or limits the failure, such as a test or a review. |

## Rules

### Naming and location

- Save 1 plan as `tasks/<module-id>.md`, with the module ID of its specification.
- Number the phases from 1.
- Number the tasks from 1 across the whole plan, in the order of work.

### Format and content

- Use the sections in the template order.
- Do not add other top-level sections. Add a new section to this convention before you use it in a plan.

### Workflow

- Deliver a plan with these steps:
  1. Write the plan, and write each task as an Issue draft in `tasks/.todo.md` with the [Template](github-issues.md#template).
  2. Show the plan and the drafts to the maintainer in the chat.
  3. When the maintainer agrees with the plan in the chat, create the milestone and the Issues, as the [Links and tracking](#links-and-tracking) rules state.
  4. After every Issue appears in the GitHub Project with the `Todo` status and in the milestone, replace each task title with its Issue link, add the tracking sentence of the [Task list](#task-list), and delete `tasks/.todo.md`.
  5. Change the status to `Approved`, as the [Status and approval](#status-and-approval) rules state.
  6. Open 1 PR with the approved plan and the title `docs(<scope, if any>): approve <capability> plan`. Write the capability name in lowercase, except for names and abbreviations.

### Links and tracking

- After the maintainer agrees with the plan, create or reuse the milestone of the plan, as the [Workflow](github-milestones.md#workflow) rules of the milestone convention state.
- After the milestone exists, create 1 Issue from each draft, as the [Workflow](github-issues.md#workflow) rules of the Issue convention state.

### Status and approval

- Write a plan only for a specification with the `Approved` status.
- Write a new plan with the `Draft` status.
- After every Issue of the plan is in the GitHub Project and in the milestone, change the status to `Approved`.
- Let only the maintainer approve a plan. The approval is the merge of the PR that adds the plan with the `Approved` status. The agreement of the maintainer in the chat does not approve the plan.
- Change the status to `Complete` in the PR of the final Prove task, when every item of that task passes.

### Changes

- If an `Approved` plan adds, removes, or reorders tasks, update the plan, the Issues, and the milestone in 1 PR with the title `docs(<scope, if any>): reapprove <capability> plan`, after the maintainer agrees in the chat. A change that only corrects wording does not need this process.
- In the PR that closes the last open Issue of a phase, follow these steps:
  1. Make sure that each outcome of the checkpoint of the phase holds.
  2. Mark the checkpoint items as complete.
- Apply a change of this convention only to new plans and to plans that a later PR changes. In that PR, convert the whole plan to the current template.

## Differences from the planning-and-task-breakdown skill

The [Planning and Task Breakdown](../../.agents/skills/planning-and-task-breakdown/SKILL.md) skill gives a generic plan format and task list, but this convention applies where the skill and this convention differ:

- Follow the [Naming and location](#naming-and-location) rules for the plan file. The skill saves 1 plan as `tasks/plan.md`.
- Use only the sections that the [Format and content](#format-and-content) rules allow. The skill adds a section for open questions.
- Follow the [Task list](#task-list) and [Workflow](#workflow) rules for the tasks and the Issue drafts. By default, the skill keeps the task list in `tasks/todo.md`, and it uses an external tracker only when the project names one.
- Follow the [Dependency graph](#dependency-graph) rules for the order of work, and the [Links and tracking](github-issues.md#links-and-tracking) rules of the Issue convention for the task dependencies. The skill shows the dependency graph as a text tree.
