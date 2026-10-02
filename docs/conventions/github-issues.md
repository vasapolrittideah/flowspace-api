# GitHub Issue conventions

This convention defines the GitHub Issue for one task, the gap comment that an ordinary task can need, and the result comment of a final Prove task. A task is one piece of work that one PR completes. A module is one capability that the [module specification convention](module-specs.md) defines, and a module plan lists the tasks of one module, as the [module plan convention](module-plans.md) defines. The final Prove task is the last task of a module plan, and it proves the approved specification. Every other task is an ordinary task. A gap is an item in `Acceptance criteria` or `Verification` that failed or did not run. Work for later is work that a task or a PR does not do but shows to be needed, such as a gap or a step that the goal still needs. A follow-up task is one line that names work for later.

## Template

An Issue has a title, a module, Description, Acceptance criteria, Verification, and Files likely touched.

```markdown
Module: `<module ID, if any>`

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

### Module

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

### Sections and wording

- Use the body fields in the template order, with the same spelling and capitalization.

### Workflow

- Write one Issue for each task.
- Before you create an Issue for a module plan, compare its title, body, and planned blockers with the approved specification and the module plan.
- After you create an Issue, apply the [Issue labels](github-labels.md), and add the Issue to the [flowspace-api GitHub Project](https://github.com/users/vasapolrittideah/projects/4) with the `Todo` status.
- When you start a task, set the Project status of its Issue to `In Progress`. After the Issue closes, make sure that its status is `Done`.
- When the local checks and CI of the PR pass, check each passed item in `Acceptance criteria` and `Verification`. Leave each gap unchecked. Do this before you tell the maintainer that the PR is ready.

### Links and records

- Record each blocking Issue as one native GitHub `Blocked by` relationship. Use these relationships as the dependency list, and do not list dependencies in the body.
- If the Issue belongs to a module plan, assign the milestone of the plan, as the [milestone rules](github-milestones.md) state.

### Changes

- If the approved specification or the module plan changes, update the body of each open Issue that the change affects.
- Apply a change of this convention to new Issues and comments, and to open Issues when you edit them. Do not edit closed Issues to follow it.

## Follow-up task format

A PR description, a gap comment, and a result comment list work for later in this format.

- Write one line for each piece of work for later, even when there is only one.
- Put the lines with an Issue first, in ascending Issue number. Then put the lines without an Issue.
- Before you create an Issue for the work, ask the maintainer. If the maintainer approves, create the Issue as this convention states. If the maintainer does not approve it, or the session has no chat, write the work without an Issue number.

A follow-up task has an Issue number and remaining work.

```markdown
- #<number of an open Issue, if any>: <remaining work>.
```

### Issue number

- If the work has an open Issue, write its number. If the work has no open Issue, omit the number, the `#`, and the colon, such as `- Rename the hexagonal convention file.`
- Do not put `Closes`, `Fixes`, or `Resolves` before the number. GitHub would close the Issue when the PR merges.

### Remaining work

- Write one sentence that names the work. GitHub shows only the number of an Issue, so the sentence must name the work.

## Gap comment

The PR that closes an Issue records the changes and the CI result, so an ordinary task has no comment by default. Post a gap comment only when the Issue has a gap or work for later that a reader of the Issue needs to know.

- Post the comment on the Issue before you tell the maintainer that the PR is ready.
- Do not add the PR, CI results, or measurements to the comment.
- Do not include secrets or test account data.

A gap comment has Gaps and Follow-up tasks.

```markdown
## Gaps

- <gap, if any>: <cause>. <next action>

## Follow-up tasks

- <follow-up task, if any>
```

### Gaps

- Write one bullet for each gap.
- If there is no gap, omit the section. Do not write `n/a`.

### Follow-up tasks

- Write each piece of work for later in the [follow-up task format](#follow-up-task-format).
- If there is no work for later, omit the section. Do not write `n/a`.

## Final Prove task

The final Prove task checks the approved specification through tests and review. Its Issue uses the [Issue template](#template), and its result goes in a result comment.

- Title the Issue `Prove <capability> against its specification`, with the capability name from the title of the specification.
- In `Description`, name the final test scope and each cross-service check that another Issue owns. Link to that Issue instead of repeating its work.
- In `Acceptance criteria`, require a test or a recorded cluster check for each success criterion of the specification and for each threat ID in its `Testing strategy`.
- In `Acceptance criteria`, add an item for each public, private, failure, and cross-service path that the module has.
- If the module has public REST routes, require Bruno smoke tests under `tests/smoke/bruno/` that run against a running service, and add a `task smoke:bruno` item to `Verification`.
- In `Verification`, add one item for each integration, smoke, contract, and generation check that applies to the module.
- In `Files likely touched`, include `docs/specs/<module-id>.md`, `tasks/<module-id>.md`, and the likely test paths.
- When every item passes, mark the specification `Implemented` and the plan `Complete` in the PR of the task, and record the PR in the plan.
- If an item fails or does not run, record the gap in the result comment. Use `Refs` instead of `Closes` for the Issue in the PR, and keep the Issue and the final plan checkpoint open until the gap is resolved.

### Result comment

The result comment records the evidence that CI does not keep: local checks, gaps, the module closure decision, and work for later.

- Post the comment on the Issue before you tell the maintainer that the PR is ready.
- Complete every section, and separate the lines under `Result` with blank lines.
- Do not add a date, because GitHub shows when the comment was posted.
- Do not repeat the Verification checklist or CI measurements.
- Do not include secrets or test account data.

A result comment has a status, an outcome, local checks, a PR line, gaps, module closure, and follow-up tasks.

```markdown
## Result

Status: <state of the module proof>

Outcome: <result of the task>

Local checks: <checks that CI does not run and their results>

PR: #<PR number>. CI <CI result> in [run <run ID>](https://github.com/vasapolrittideah/flowspace-api/actions/runs/<run ID>).

## Gaps

<gaps>

## Module closure

<specification and plan statuses>

## Follow-up tasks

<follow-up tasks>
```

#### Status

- Write `Complete` when every item in `Acceptance criteria` and `Verification` passes. Otherwise, write `Blocked`.
- If you write `Blocked`, keep the specification and plan statuses unchanged.
- Do not use other values.

#### Outcome

- Write one sentence.

#### Local checks

- For each check, state the command, its passed and total counts, and the local stack that it ran against, such as `task smoke:bruno` passed 36/36 requests against k3d flowspace-local with Mailpit. If the check has no counts, state its result instead.
- Separate several checks with semicolons.
- If no local check applies, write `Local checks: n/a`.
- Do not include the checks that the [CI checks](../../.github/workflows/ci.yml) run, such as `task check:task` or the Buf commands.

#### PR line

- Write `passed` or `failed` for the CI result, and link the CI run of the last commit of the PR.

#### Result gaps

- Write one bullet for each gap in the format of the gap comment, such as `- <gap>: <cause>. <next action>`.
- If there is no gap, write `n/a`.

#### Module closure

- Start with the sentence "The specification is `<status>` and the plan is `<status>`."
- If a success criterion has only partial proof, state why the module can still close.

#### Result follow-up tasks

- Write each piece of work for later in the [follow-up task format](#follow-up-task-format).
- If there is no work for later, write `n/a`.

## Differences from the planning-and-task-breakdown skill

The [`planning-and-task-breakdown` skill](../../.agents/skills/planning-and-task-breakdown/SKILL.md) gives each task a structure for a task list or a tracker. This convention applies where the two differ:

- Write the task as a GitHub Issue with the [Issue template](#template). The skill writes `Description`, `Acceptance criteria`, and other fields as bold labels in a `## Task` section.
- Record dependencies as native `Blocked by` relationships, as the [link rules](#links-and-records) state. The skill lists them in a `Dependencies` field.
- Omit an estimated scope. The skill sizes each task by its number of files.
- End `Verification` with the fixed items in the [Verification rules](#verification). The skill uses `Tests pass`, `Build succeeds`, and `Manual check` labels.

## Examples

[Issue #303](https://github.com/vasapolrittideah/flowspace-api/issues/303) shows an ordinary task Issue that follows this convention.

This result comment shows a final Prove task that passed with a partial proof:

```markdown
## Result

Status: Complete

Outcome: Identity provider login passed its specification through public REST, provider callback, PostgreSQL, Mailpit, private session check, and worker retention tests.

Local checks: `task smoke:bruno` passed 44/44 requests against k3d flowspace-local with Mailpit and both provider clients.

PR: #266. CI passed in [run 36831313638](https://github.com/vasapolrittideah/flowspace-api/actions/runs/36831313638).

## Gaps

n/a

## Module closure

The specification is `Implemented` and the plan is `Complete`. No live login ran with a real Google or GitHub account, because the specification allows live provider tests only with disposable accounts and no success criterion requires them.

## Follow-up tasks

n/a
```
