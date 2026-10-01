# GitHub Issue conventions

This convention defines the Issue body for one task, the gap comment for an ordinary task, and the result comment for a final Prove task.

## Template

An Issue has a title that states the task outcome, and the body fields shown below.

```markdown
Module: `<module ID>`

Estimated scope: <expected size of the task>.

## Description

<task outcome and scope>

## Acceptance criteria

- [ ] <outcome that can be checked on its own>

## Verification

- [ ] Run `<command>` to check <behavior that the command checks>.

## Files likely touched

- `<source, test, contract, or configuration path>`
```

### Title

- Keep the title consistent with the approved specification, module plan, and task scope.

### Module

- Copy the module ID from the approved specification.

### Estimated scope

- Write one word, such as `Small`, `Medium`, or `Large`.

### Description

- Put prerequisites without an Issue number in a separate paragraph.

### Acceptance criteria

- Write each outcome as a `- [ ]` item.
- Use as many items as the task needs.

### Verification

- Write each check as a `- [ ]` item.
- Keep inspection of a command's output in the same item when they form one check. Put separate manual checks in separate items.
- Include `task check:task`, `task git:diff:check`, and CI review in every list. If Markdown changes, include `task markdown:check`.
- Add commands that check the task's behavior.

### Files likely touched

- For generated output, list only its folder, with a trailing slash and `(generated output)`, such as `gen/go/flowspace/identity/v1/` (generated output).
- Do not list generated file names.

## Rules

- Follow the workflow and authority in the [agent instructions](../../AGENTS.md) when creating Issues.
- Use one Issue for each task. Use the body fields in the template order, with the same spelling and capitalization.
- Keep the body consistent with the approved specification, module plan, and task scope.
- Before creating an Issue, compare its title, body, acceptance criteria, verification, file list, and planned blockers with the approved specification and module plan.
- After creating an Issue, apply the [Issue labels](github-labels.md), add it to the repository GitHub Project with `Todo` status, and record its blockers as native GitHub `Blocked by` relationships. Assign the [plan milestone](github-milestones.md) to each Issue in the plan's numbered Task list.
- Add one native GitHub `Blocked by` relationship for each blocking Issue. Use these relationships as the dependency list.

## Follow-up task format

A follow-up task is an open Issue for work that a PR or an Issue leaves for later. A PR description, a gap comment, and a result comment list follow-up tasks in this format.

```markdown
- #<number of an open Issue>: <remaining work>.
```

### Issue number

- Write one bullet per Issue, even when there is only one.
- List only open Issues. Create an Issue for later work before you list it, or leave the work out.
- Do not put `Closes`, `Fixes`, or `Resolves` before the number. GitHub would close the Issue when the PR merges.

### Remaining work

- Write one sentence. GitHub shows only the number, so the sentence names the work.

## Gap comment

After verification, check each completed item in the Issue body. Leave failed and unrun items unchecked. The pull request (PR) that closes the Issue records the changes and the CI result, so an ordinary Issue has no comment by default.

Post a comment only when the PR does not show something that a reader of the Issue needs: an unchecked item or a gap that needs a follow-up Issue. Do not add the PR, CI results, or measurements to the comment. Do not include secrets or test account data.

```markdown
## Gaps

- <unchecked item>: <cause>. <next action>

## Follow-up tasks

- #<number of an open Issue>: <remaining work>.
```

### Gaps

- If no item is unchecked, omit the section. Do not write `n/a`.

### Follow-up tasks

- Write each follow-up Issue in the [follow-up task format](#follow-up-task-format).
- If there is no follow-up Issue, omit the section. Do not write `n/a`.

## Final Prove task

The final task in each module plan checks the approved specification through tests and review. Use the Issue template above.

### Issue rules

- Title the Issue `Prove <capability> against its specification`. Name the capability so readers can distinguish it from other modules.
- In Description, name the final test scope and any cross-service checks owned by another Issue. Link to that Issue instead of repeating its work.
- In Acceptance criteria, require tests for every success criterion and applicable threat.
- Add acceptance criteria for the module's public, private, failure, and cross-service paths when they apply. Name the outcomes that still need proof.
- For a module with public REST routes, require Bruno smoke tests under `tests/smoke/bruno/` against a running service. Run `task smoke:bruno` and record the result in the result comment.
- If a check fails or is missing, record the gap and its follow-up Issue in the result comment. Keep the task and final plan checkpoint open until the gap is resolved.
- Add separate Verification items for the module's applicable integration, smoke, contract, and generation checks. Check completed items in the Issue body.
- Add `Blocked by` relationships for actual blocking Issues. Include `docs/specs/<module-id>.md`, `tasks/<module-id>.md`, and likely test paths in Files likely touched.
- When every criterion passes, mark the specification `Implemented` and the plan `Complete`. Record the final PR in the plan.

### Result comment

After verification, check each completed item in the Issue body and post a result comment. The comment records the evidence that CI does not keep: local checks, gaps, the module closure decision, and follow-up work.

Separate the lines under `Result` with blank lines, and complete every section. Do not add a date, because GitHub shows when the comment was posted. Do not repeat the Verification checklist or CI measurements. Do not include secrets or test account data.

```markdown
## Result

Status: <state of the module proof>

Outcome: <result of the task>

Local checks: <checks that CI does not run and their results>

PR: #<PR number>. CI <CI result> in [run <run ID>](https://github.com/vasapolrittideah/flowspace-api/actions/runs/<run ID>).

## Gaps

<failed or unrun items, their causes, and next actions>

## Module closure

<specification and plan statuses>

## Follow-up tasks

<open Issues for later work>
```

#### Status

- Write `Complete` only when all required checks pass.
- If a gap prevents closure, write `Blocked`, and keep the specification and plan statuses unchanged.

#### Outcome

- Write one sentence.

#### Local checks

- For each check, state the command, its passed and total counts, and the local stack it ran against, such as `task smoke:bruno` passed 36/36 requests against k3d flowspace-local with Mailpit.
- Separate several checks with semicolons.
- If no local check applies, write `Local checks: n/a`.
- Do not include checks that CI runs.

#### PR

- Write `passed` or `failed` for the CI result, and link the CI run.

#### Result gaps

- For each failed or unrun item, leave its checkbox clear in the Issue body, and write a bullet that names the item, cause, and next action.
- If there is nothing to report, write `n/a`.

#### Module closure

- If a success criterion has only partial proof, state why the module can still close.

#### Result follow-up tasks

- Write each follow-up Issue in the [follow-up task format](#follow-up-task-format).
- If there is no follow-up Issue, write `n/a`.

## Examples

The [Workspace creation Issue](https://github.com/vasapolrittideah/flowspace-api/issues/48) shows a complete Issue using this template.
