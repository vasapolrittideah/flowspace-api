# GitHub Issue conventions

This convention defines the GitHub Issue for one task. A task is one piece of work that one PR completes. A module is one capability that the [module specification convention](module-specs.md) defines, and a module plan lists the tasks of one module, as the [module plan convention](module-plans.md) defines. The final Prove task is the last task of a module plan, and it proves the approved specification. Every other task is an ordinary task. A gap is an item in `Acceptance criteria` or `Verification` that failed or did not run.

## Template

An Issue has a title, a module ID, Description, Acceptance criteria, Verification, and Files likely touched.

```markdown
Module ID: `<module ID, if any>`

## Description

<task outcome and scope>

## Acceptance criteria

- [ ] <outcome that can be checked on its own>

## Verification

- [ ] <check and the behavior that it checks>

## Files likely touched

- `<source, test, contract, or configuration path>`
```

### Title

- Write the title in the title field of the Issue, outside the body.
- Write a verb phrase in the imperative mood and in sentence case, without a final period, such as "Trace the Workspace session check into Identity".
- State the outcome of the task, not its steps.
- Keep the title consistent with the approved specification, the module plan, and the task scope.

### Module ID

- Copy the module ID from the approved specification.
- If the Issue does not belong to a module plan, omit the line.

### Description

- Write the description as paragraphs. Start with the outcome, and then state the scope.
- If the task has a prerequisite without an Issue, such as an approved decision, put the prerequisite in its own paragraph.

### Acceptance criteria

- Write at least one `- [ ]` item.
- State each outcome as a fact in the present tense, such as "The Identity `identity_session_check` line has the same request ID and trace ID as the Workspace request."
- If the task delivers a success criterion of the specification, write an item for that criterion.

### Verification

- Write each check as a `- [ ]` item.
- Start a command check with `Run`, the command in backticks, and `to check`, and then state the behavior that the command checks. Start a manual check with its action, such as "Stop Alloy with `kubectl scale`".
- Keep the inspection of a command's output in the same item as the command when they form one check. Put each separate manual check in its own item.
- Put the checks of the task first. Then end the list with these items, in this order, with this exact text:
  1. Run `task check:task` to check formatting, lint, tests, coverage, and vulnerabilities.
  2. Run `task git:diff:check` to check whitespace in the diff.
  3. Run `task markdown:check` to check changed Markdown.
  4. Review the CI results on the PR.
- If the task changes no Markdown file, omit the `task markdown:check` item.

### Files likely touched

- Write one item for each path that the task likely changes.
- For generated output, list only its folder, with a trailing slash, followed by `(generated output)`, such as `` `gen/go/flowspace/identity/v1/` (generated output) ``.
- Do not list generated file names.

## Rules

### Format and content

- Use the body fields in the template order, with the same spelling and capitalization.

### Workflow

- Write one Issue for each task.
- Before you create an Issue for a module plan, compare its title, body, and planned blockers with the approved specification and the module plan.
- After you create an Issue, apply the [Issue labels](github-labels.md), and add the Issue to the [flowspace-api GitHub Project](https://github.com/users/vasapolrittideah/projects/4) with the `Todo` status.
- When you start a task, set the Project status of its Issue to `In Progress`. After the Issue closes, make sure that its status is `Done`.
- Before you tell the maintainer that the PR is ready, or that it cannot become ready, check each passed item in `Acceptance criteria` and `Verification`. Leave each gap unchecked.
- If an ordinary task has a gap, let its PR close the Issue. The PR states the gap in `Risks or limitations` and the remaining work in `Follow-up tasks`. A gap in the final Prove task keeps the Issue open, as the [Final Prove task](#final-prove-task) rules state.
- Do not post a comment with the results of a task. The PR description records them.

### Links and tracking

- Record each blocking Issue as one native GitHub `Blocked by` relationship. Use these relationships as the dependency list, and do not list blocking Issues in the body.
- If the Issue belongs to a module plan, assign the milestone of the plan, as the [milestone rules](github-milestones.md) state.

### Changes

- If the approved specification or the module plan changes, update the body of each open Issue that the change affects.
- Apply a change of this convention to new Issues, and to open Issues when you edit them. Do not edit closed Issues to follow it.

## Final Prove task

The final Prove task checks the approved specification through tests and review. Its Issue uses the [Issue template](#template), and its PR records the evidence that CI does not keep, as the [Prove task PR rules](pull-requests.md#final-prove-task-pr) state.

- Title the Issue `Prove <capability> against its specification`, with the capability name from the title of the specification.
- In `Description`, name the final test scope and each cross-service check that another Issue owns. Link to that Issue instead of repeating its work.
- In `Acceptance criteria`, require a test, or a local check that the PR description records, for each success criterion of the specification and for each threat ID in its `Testing strategy`.
- In `Acceptance criteria`, add an item for each public, private, failure, and cross-service path that the module has.
- If the module has public REST routes, require Bruno smoke tests under `tests/smoke/bruno/` that run against a running service, and add a `task smoke:bruno` item to `Verification`.
- In `Verification`, add one item for each integration, smoke, contract, and generation check that applies to the module.
- In `Files likely touched`, include `docs/specs/<module-id>.md`, `tasks/<module-id>.md`, and the likely test paths.
- When every item passes, mark the specification `Implemented` and the plan `Complete` in the PR of the task.
- If an item fails or does not run, keep the specification and plan statuses unchanged. Keep the Issue and the final plan checkpoint open until the gap is resolved. The [Prove task PR rules](pull-requests.md#final-prove-task-pr) state how the PR refers to the Issue.

## Differences from the planning-and-task-breakdown skill

The [`planning-and-task-breakdown` skill](../../.agents/skills/planning-and-task-breakdown/SKILL.md) gives each task a structure for a task list or a tracker. This convention applies where the two differ:

- Write the task as a GitHub Issue with the [Issue template](#template). The skill writes `Description`, `Acceptance criteria`, and other fields as bold labels in a `## Task` section.
- Record dependencies as native `Blocked by` relationships, as the [link rules](#links-and-tracking) state. The skill lists them in a `Dependencies` field.
- Omit an estimated scope. The skill sizes each task by its number of files.
- End `Verification` with the fixed items in the [Verification rules](#verification). The skill uses `Tests pass`, `Build succeeds`, and `Manual check` labels.

## Examples

[Issue #303](https://github.com/vasapolrittideah/flowspace-api/issues/303) shows an ordinary task Issue that follows this convention.
