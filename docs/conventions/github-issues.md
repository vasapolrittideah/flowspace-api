# GitHub Issue conventions

This convention defines the format and life cycle of the GitHub Issue for 1 task.

[ADR-0040](../adr/0040-automatic-builds-run-one-subagent-for-each-ready-issue.md) and [ADR-0041](../adr/0041-maintainers-answer-automatic-builds-in-the-chat.md) state the decisions for automatic builds.

## Template

An Issue has a title, a module ID, [Description](#description), [Acceptance criteria](#acceptance-criteria), [Verification](#verification), and [Files likely touched](#files-likely-touched).

```markdown
<Issue title>

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

- If the Issue does not belong to a module plan, omit the line.
- Copy the module ID from the approved specification.

### Description

- Write the description as paragraphs. Start with the outcome, and then state the scope.
- If the task has a prerequisite without an Issue, such as an approved decision, put the prerequisite in its own paragraph.

### Acceptance criteria

- Write at least 1 `- [ ]` item.
- State each outcome as a fact in the present tense, such as "The Identity `identity_session_check` line has the same request ID and trace ID as the Workspace request."
- If the task delivers a success criterion of the specification, write an item for that criterion.

### Verification

- Write each check as a `- [ ]` item.
- Start a command check with "Run", the command in backticks, and "to check", and then state the behavior that the command checks.
- Start a manual check with its action, such as "Stop Alloy with `kubectl scale`".
- If the task changes no Markdown file, omit the `task markdown:check` item.
- Keep the inspection of a command's output in the same item as the command when they form 1 check. Put each separate manual check in its own item.
- Put the checks of the task first. Then end the list with these items, in this order, with this exact text:
  1. Run `task check:task` to check formatting, lint, tests, coverage, and vulnerabilities.
  2. Run `task git:diff:check` to check whitespace in the diff.
  3. Run `task markdown:check` to check changed Markdown.
  4. Review the CI results on the PR.

### Files likely touched

- Write 1 item for each path that the task likely changes.
- For generated output, list only its folder, with a trailing slash, followed by `(generated output)`, such as `` `gen/go/flowspace/identity/v1/` (generated output) ``.
- Do not list generated file names.

## Rules

### Format and content

- Use the body fields in the template order, with the same spelling and capitalization.
- Start each agent comment with a marker, such as `<!-- automatic-build -->`.
- End each agent comment with the attribution line `🤖 Generated with [Claude Code](https://claude.com/claude-code)`.
- Write each agent comment in English, as the [Markdown and English prose](markdown-and-english-prose.md) conventions state.
- Do not add an estimated scope to an Issue.

### Workflow

- Write 1 Issue for each task.
- Before you create an Issue for a module plan, compare its title, body, and planned blockers with the approved specification and the module plan.
- Before you tell the maintainer that the PR is ready, or that it cannot become ready, check each passed item in [Acceptance criteria](#acceptance-criteria) and [Verification](#verification). Leave each gap unchecked.
- Do not post a comment with the results of a task. The PR description records them. The comments of an [Automatic build](#automatic-build) record the run, not the results.

### Links and tracking

- After you create an Issue, apply the labels that the [GitHub label](github-labels.md) conventions state.
- After you create an Issue, add it to the [flowspace-api](https://github.com/users/vasapolrittideah/projects/4) GitHub Project.
- Record each blocking Issue as 1 native GitHub `Blocked by` relationship. Use these relationships as the dependency list, and do not list blocking Issues in the body.
- If the Issue belongs to a module plan, assign the milestone of the plan, as the [GitHub milestone](github-milestones.md) conventions state.

### Status and approval

- After you create an Issue, set its Project status to `Todo`.
- When you start a task, set the Project status of its Issue to `In Progress`.
- After the Issue closes, make sure that its status is `Done`.
- If an ordinary task has a gap, let its PR close the Issue, and record the gap as the [Risks or limitations](pull-requests.md#risks-or-limitations) and [Follow-up tasks](pull-requests.md#follow-up-tasks) rules state. A gap in the final Prove task keeps the Issue open, as the [Final Prove task](#final-prove-task) rules state.

### Changes

- If the approved specification or the module plan changes, update the body of each open Issue that the change affects.
- Apply a change of this convention to new Issues, and to open Issues when you edit them. Do not edit closed Issues to follow it.

## Final Prove task

The final Prove task checks the approved specification through tests and review. Its PR records the evidence that CI does not keep, as the [Final Prove task PR](pull-requests.md#final-prove-task-pr) rules state.

- When every item passes, change the statuses of the specification and the plan in the PR of the task, as the [Status and approval](module-specs.md#status-and-approval) rules of the module specification convention and the [Status and approval](module-plans.md#status-and-approval) rules of the module plan convention state.
- If an item fails or does not run, keep the specification and plan statuses unchanged. Keep the Issue and the final plan checkpoint open until the gap is resolved. The [Final Prove task PR](pull-requests.md#final-prove-task-pr) rules state how the PR refers to the Issue.

The Issue of the final Prove task has a title, a module ID, [Description](#description), [Acceptance criteria](#acceptance-criteria), [Verification](#verification), and [Files likely touched](#files-likely-touched).

```markdown
Prove <capability name> against its specification

Module ID: `<module ID>`

## Description

<final test scope and cross-service checks>

## Acceptance criteria

- [ ] <test or local check that proves a part of the specification>

## Verification

- [ ] <check that applies to the module>

## Files likely touched

- `<specification, plan, or test path>`
```

### Final Prove title

- Use the capability name from the title of the specification.

### Final Prove module ID

- Follow the [Module ID](#module-id) rules.

### Final Prove description

- Name the final test scope and each cross-service check that another Issue owns. Link to that Issue instead of repeating its work.
- Follow the other [Description](#description) rules.

### Final Prove acceptance criteria

- Require a test, or a local check that the PR description records, for each success criterion of the specification and for each threat ID in its [Testing strategy](module-specs.md#testing-strategy).
- Add an item for each public, private, failure, and cross-service path that the module has.
- If the module has public REST routes, require Bruno smoke tests under `tests/smoke/bruno/` that run against a running service.
- Follow the other [Acceptance criteria](#acceptance-criteria) rules.

### Final Prove verification

- If the module has public REST routes, add a `task smoke:bruno` item.
- Add 1 item for each integration, smoke, contract, and generation check that applies to the module.
- Follow the other [Verification](#verification) rules.

### Final Prove files likely touched

- Include `docs/specs/<module-id>.md`, `tasks/<module-id>.md`, and the likely test paths.
- Follow the other [Files likely touched](#files-likely-touched) rules.

## Automatic build

These rules apply to each Issue that an automatic build starts. The [Automatic modes](../../.agents/commands/build.md#automatic-modes) section of the build command gives the steps of the run. Each agent comment follows the [Format and content](#format-and-content) rules.

- Before the subagent of an Issue starts, set the Project status of the Issue to `In Progress`, and add a [Start comment](#start-comment). Add a new start comment for each start.
- When the build of an Issue stops, add a [Stop comment](#stop-comment), and set the Project status to `Needs human`. If the branch of the start comment has no open PR, also change the marker of the start comment to `<!-- automatic-build-ended -->`. If the branch has an open PR, also convert the PR to a draft with `gh pr ready --undo`, so that GitHub blocks its merge.
- When the maintainer answers a stop in the chat, record the answer with these steps, as [ADR-0041](../adr/0041-maintainers-answer-automatic-builds-in-the-chat.md) states:
  1. Add the answer to the [Stop comment](#stop-comment).
  2. If the Issue has no open PR, set the Project status to `Todo`, so that a later run starts the Issue again. If the PR is open, set the status to `In Progress`, so that a later run continues the work on that PR.

### Start comment

A start comment has a marker, a notice, a branch line, and an attribution line. It shows that an automatic build works on the Issue.

```markdown
<!-- automatic-build -->
An automatic build started this Issue.

Branch: <branch name, if any>

🤖 Generated with [Claude Code](https://claude.com/claude-code)
```

#### Marker

- While the build works, write `<!-- automatic-build -->`. After the build stops without an open PR, write `<!-- automatic-build-ended -->`.

#### Notice

- Write the fixed sentence of the template.

#### Branch line

- When the subagent creates the branch, add the line with the exact branch name.
- Until the branch exists, omit the line.

#### Attribution line

- Write the fixed line of the template.

### Stop comment

A stop comment has a marker, a reason line, a question, an answer line, and an attribution line. It tells the maintainer why the build stopped, and it keeps the answer for a later run.

```markdown
<!-- automatic-build-stop -->
Reason: <reason that the build stopped>

<question for the maintainer>

Answer: <answer of the maintainer, if any>

🤖 Generated with [Claude Code](https://claude.com/claude-code)
```

#### Stop marker

- Write `<!-- automatic-build-stop -->`.

#### Reason line

- When the specification or the plan must change, write exactly `Reason: specification or plan change`.
- State the step that stopped and the cause in 1 sentence.

#### Question

- Ask 1 question that the maintainer can answer in the chat.

#### Answer line

- When the maintainer answers in the chat, edit the comment and add the answer in English.
- Until the maintainer answers, omit the line.

#### Stop attribution line

- Write the fixed line of the template.

## Differences from the planning-and-task-breakdown skill

The [Planning and Task Breakdown](../../.agents/skills/planning-and-task-breakdown/SKILL.md) skill gives each task a structure for a task list or a tracker, but this convention applies where the skill and this convention differ:

- Follow the [Template](#template) and the [Workflow](#workflow) rules. The skill writes `Description`, `Acceptance criteria`, and other fields as bold labels in a `## Task` section.
- Follow the dependency rules in [Links and tracking](#links-and-tracking). The skill lists the dependencies in a `Dependencies` field.
- Follow the [Format and content](#format-and-content) rules for the estimated scope. The skill sizes each task by its number of files.
- Follow the [Verification](#verification) rules. The skill uses the "Tests pass", "Build succeeds", and "Manual check" labels.

## Examples

This Issue is an ordinary task Issue that follows this convention:

[Issue #303](https://github.com/vasapolrittideah/flowspace-api/issues/303)
