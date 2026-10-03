# Module plan conventions

This convention defines the file, the format, and the status of one module plan in `tasks/`. A module plan breaks an approved [module specification](module-specs.md) into tasks, and each task becomes one GitHub Issue, as the [GitHub Issue convention](github-issues.md) defines. A phase is a group of tasks. A checkpoint lists the outcomes that a reviewer checks after the tasks of a phase are done.

## Template

A plan has a title, a module ID, a status line, `Overview`, `Architecture decisions`, `Dependency graph`, `Task list`, and `Risks and controls`.

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

Tasks are tracked in the [flowspace-api GitHub Project](https://github.com/users/vasapolrittideah/projects/4) under the [<capability name> milestone](<milestone URL>).

### Phase <number>: <phase name>

- Task <number>: <task title or Issue link>

### Checkpoint: <phase name>

- [ ] <outcome that a reviewer checks>

## Risks and controls

| Risk | Impact | Control |
| --- | --- | --- |
| <failure that can happen> | <what users or the system lose> | <step that prevents or limits it> |
````

### Title

- Copy the capability name from the title of the approved specification, without the `Spec:` prefix.

### Module ID

- Copy the module ID from the approved specification, in backticks.

### Status line

- Write `Draft`, `Approved`, or `Complete`, without a final period, as the [status rules](#status-and-approval) state.

### Overview

- Write the overview as paragraphs. Start with the outcome of the module.
- Include the sentence "The plan follows the approved specification.", and link "the approved specification" to the specification file.

### Architecture decisions

- Write one bullet for each decision that sets a task boundary, such as a shared package, a dependency, or a migration.
- Link the ADR that a decision applies, if any.
- Treat an open architecture proposal as undecided. Do not write a decision that depends on one.

### Dependency graph

- Draw the graph as a Mermaid `flowchart TD` diagram in a `mermaid` code block.
- Write an edge `A --> B` when the work of `B` cannot start before the work of `A` is done.
- Give each node a one-word ID in PascalCase and a label that names its work, such as `Contract[Public recovery contract]`.
- End the graph at one node for the final Prove task.
- Do not put task numbers in node labels. The graph can show work that is not a task, such as an approved decision, and native `Blocked by` relationships record the task dependencies.

### Task list

- Keep the fixed sentence that links the GitHub Project and the milestone of the plan. While the plan is `Draft`, omit the sentence, because the milestone does not exist yet.
- Group the tasks in phases, numbered from 1, under `### Phase <N>: <name>` headings. Name each phase with a noun phrase in sentence case, such as "Export and storage".
- Number the tasks from 1 across the whole plan, in the order of work. Put the final Prove task alone in the last phase.
- While the plan is `Draft`, write each task as `- Task <N>: <task title>`. After its Issue exists, replace the task title with a link to the Issue, and use `#<number> <Issue title>` as the link text.
- After the tasks of each phase, write `### Checkpoint: <phase name>` with one `- [ ]` item for each outcome. Name the checkpoint of the last phase `### Checkpoint: Complete`.
- Check the items of a checkpoint in the PR that closes the last open Issue of its phase, after you verify each outcome.
- Do not keep a task checklist apart from the task lines, because the Issues and the GitHub Project hold the status of each task.

### Risks and controls

- Write a table with one row for each material risk, as the module specification convention defines a material risk.

#### Risks table

- Write the columns in this order.

| Column | How to write |
| --- | --- |
| `Risk` | The failure that can happen, as one sentence. |
| `Impact` | What users or the system lose when the failure happens. |
| `Control` | The step in the plan that prevents or limits the failure, such as a test or a review. |

## Rules

### Naming and location

- Save one plan as `tasks/<module-id>.md`, with the module ID of its specification.

### Format and content

- Use the sections in the template order.
- Do not add other top-level sections. Add a new section to this convention before you use it in a plan.

### Workflow

- Write a plan only for a specification with the `Approved` status.
- Write the plan with the `Draft` status, and write each task as an Issue draft in `tasks/.todo.md` with the [Issue template](github-issues.md#template). Show the plan and the drafts to the maintainer in the chat.
- When the maintainer agrees with the plan in the chat, create or reuse the milestone of the plan, as the [milestone rules](github-milestones.md) state. Then create one Issue from each draft, as the Issue convention states. This agreement does not approve the plan.
- After every Issue appears in the GitHub Project with the `Todo` status and in the milestone, replace each task title with its Issue link, add the fixed sentence of the task list, delete `tasks/.todo.md`, and change the status to `Approved`.
- Open one PR with the approved plan and the title `docs(<scope, if any>): approve <capability> plan`. Write the capability name in lowercase, except for names and abbreviations.

### Status and approval

- Let only the maintainer approve a plan. The approval is the merge of the PR that adds the plan with the `Approved` status.
- Change the status to `Complete` in the PR of the final Prove task, when every item of that task passes.

### Changes

- If an `Approved` plan adds, removes, or reorders tasks, update the plan, the Issues, and the milestone in one PR with the title `docs(<scope, if any>): reapprove <capability> plan`, after the maintainer agrees in the chat. A change that only corrects wording does not need this process.
- Apply a change of this convention to new plans, and to `Approved` plans that a later PR changes. In that PR, convert the whole plan to the current template. Keep each `Complete` plan in the format that it had when it was completed.

## Differences from the planning-and-task-breakdown skill

The [`planning-and-task-breakdown` skill](../../.agents/skills/planning-and-task-breakdown/SKILL.md) gives a generic plan format and task list. This convention applies where the two differ:

- Save each plan as `tasks/<module-id>.md`, so a new plan does not replace the plan of another module. The skill saves one plan as `tasks/plan.md`.
- Hold the tasks in GitHub Issues and their status in the GitHub Project, and keep only Issue drafts in `tasks/.todo.md`. The skill keeps the task list in `tasks/todo.md`.
- Record task dependencies as native `Blocked by` relationships, and show the order of work in `Dependency graph`. The skill lists the dependencies of each task in the task.
- Omit an `Open Questions` section, because the specification resolves its open questions before approval. The skill adds one.
