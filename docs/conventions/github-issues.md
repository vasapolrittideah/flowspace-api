# GitHub Issue conventions

This convention defines the Issue body for one task, the gap comment for an ordinary task, and the result comment for a final Prove task.

## Template

An Issue has a title and the body fields shown below.

```markdown
Module: `<module-id>`

Estimated scope: <expected size>.

## Description

<task outcome and scope>

## Acceptance criteria

- [ ] <outcome that can be checked on its own>

## Verification

- [ ] Run `<command>` to check <behavior>.

## Files likely touched

- `<source, test, contract, or configuration path>`
```

- Title: The task outcome.
- Module: Use the module ID from the approved specification.
- Estimated scope: State the expected size of the task.
- Description: State the task outcome and scope. Put prerequisites without an Issue number in a separate paragraph.
- Acceptance criteria: Independently checkable outcomes, each as a `- [ ]` item.
- Verification: Commands and the behavior each command checks, each as a `- [ ]` item.
- Files likely touched: Source, test, contract, and configuration paths. Generated output appears as a folder with a trailing slash and `(generated output)`, such as `gen/go/flowspace/identity/v1/` (generated output).

## Rules

- Follow the workflow and authority in the [agent instructions](../../AGENTS.md) when creating Issues.
- Use one Issue for each task. Use the body fields in the template order, with the same spelling and capitalization.
- State the task outcome in the Issue title. Keep the title and body consistent with the approved specification, module plan, and task scope.
- Before creating an Issue, compare its title, body, acceptance criteria, verification, file list, and planned blockers with the approved specification and module plan.
- After creating an Issue, apply the [Issue labels](github-labels.md), add it to the repository GitHub Project with `Todo` status, and record its blockers as native GitHub `Blocked by` relationships. Assign the [plan milestone](github-milestones.md) to each Issue in the plan's numbered Task list.
- Use as many acceptance criteria items as the task needs.
- Include `task check:task`, `task git:diff:check`, and CI review in every Verification list. Include `task markdown:check` when Markdown changes. Add commands that check the task's behavior.
- Keep inspection of a command's output in the same Verification item when they form one check. Put separate manual checks in separate items.
- Add one native GitHub `Blocked by` relationship for each blocking Issue. Use these relationships as the dependency list.
- For generated output, list only its folder. Do not list generated file names.

## Gap comment

After verification, check each completed item in the Issue body. Leave failed and unrun items unchecked. The pull request (PR) that closes the Issue records the changes and the CI result, so an ordinary Issue has no comment by default.

Post a comment only when the PR does not show something that a reader of the Issue needs: an unchecked item or a gap that needs a follow-up Issue.

```markdown
## Gaps

- <Unchecked item>: <cause>. <Next action.>

## Follow-up tasks

- #<issue-number>: <remaining work in one sentence>.
```

- Omit a section that has nothing to report. Do not write `n/a`.
- Write each follow-up Issue in the [Follow-up tasks format](pull-requests.md#follow-up-tasks).
- Do not add the PR, CI results, or measurements. Do not include secrets or test account data.

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

```markdown
## Result

Status: Complete | Blocked

Outcome: <State the result in one sentence.>

Local checks: <State each command that CI does not run, its passed and total counts, and the local stack it ran against.>

PR: #<number>. CI <passed | failed> in [run <run-id>](https://github.com/vasapolrittideah/flowspace-api/actions/runs/<run-id>).

## Gaps

n/a

## Module closure

<State the specification and plan statuses. If a success criterion has only partial proof, state why the module can still close.>

## Follow-up tasks

n/a
```

- Separate the lines under `Result` with blank lines. Do not add a date because GitHub shows when the comment was posted.
- Complete every section. Write `n/a` under `Gaps` or `Follow-up tasks` when there is nothing to report.
- Set `Status` to `Complete` only when all required checks pass. If a gap prevents closure, set `Status` to `Blocked` and keep the specification and plan statuses unchanged.
- For each failed or unrun item, leave its checkbox clear and add a bullet under `Gaps` that names the item, cause, and next action.
- In `Local checks`, include only checks that CI does not run, such as `task smoke:bruno` passed 36/36 requests against k3d flowspace-local with Mailpit. Separate several checks with semicolons. Write `Local checks: n/a` when no local check applies.
- Do not repeat the Verification checklist or CI measurements. Write each follow-up Issue in the [Follow-up tasks format](pull-requests.md#follow-up-tasks). Do not include secrets or test account data.

## Examples

The [Workspace creation Issue](https://github.com/vasapolrittideah/flowspace-api/issues/48) shows a complete Issue using this template.
