# GitHub Issue conventions

This convention defines the Issue body and result comment for one task.

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

| Field | Content and format |
| --- | --- |
| Title | The task outcome. |
| Module | Use the module ID from the approved specification. |
| Estimated scope | State the expected size of the task. |
| Description | State the task outcome and scope. Put prerequisites without an Issue number in a separate paragraph. |
| Acceptance criteria | Independently checkable outcomes, each as a `- [ ]` item. |
| Verification | Commands and the behavior each command checks, each as a `- [ ]` item. |
| Files likely touched | Source, test, contract, and configuration paths. Generated output appears as a folder with a trailing slash and `(generated output)`, such as `gen/go/flowspace/identity/v1/` (generated output). |

## Rules

- Follow the workflow and authority in the [agent instructions](../../AGENTS.md) when creating Issues.
- Use one Issue for each task. Use the body fields in the template order, with the same spelling and capitalization.
- State the task outcome in the Issue title. Keep the title and body consistent with the approved specification, module plan, and task scope.
- Before creating an Issue, compare its title, body, acceptance criteria, verification, file list, and planned blockers with the approved specification and module plan.
- After creating an Issue, apply the [Issue labels](github-labels.md), add it to the repository GitHub Project with `Todo` status, and record its blockers as native GitHub `Blocked by` relationships. Assign the [plan milestone](github-milestones.md) to each Issue in the plan's numbered Task list.
- Use as many acceptance criteria items as the task needs.
- Keep inspection of a command's output in the same Verification item when they form one check. Put separate manual checks in separate items.
- Add one native GitHub `Blocked by` relationship for each blocking Issue. Use these relationships as the dependency list.
- For generated output, list only its folder. Do not list generated file names.

## Task result comment

Post the result on the task Issue as a comment. Do not edit the Issue body to report results.

```markdown
## Result

Date: YYYY-MM-DD (Asia/Bangkok)
Status: Complete | Blocked
Outcome: <State the result in one sentence.>

## Verification

- `task check:task`: <Passed | Failed | Not run>. <State the reason if it did not pass.>
- `git diff origin/main...HEAD --check`: <Passed | Failed | Not run>.
- `task markdown:check`: <Passed | Failed | Not run>.
- `<task-specific command>`: <Passed | Failed | Not run>. <State the result or reason.>
- `cd tests/smoke/bruno && bru run --env local`: <Passed | Failed | Not run>. Environment: <name>.
- CI: <Passed | Failed | Not run>. <Link to the run or state the reason.>

## Measurements

| Measure | Result | Required |
| --- | --- | --- |
| Project coverage | `<actual>%` | at least 25.0% |
| Changed-line coverage | `<actual>%` or `n/a` | at least 80% when applicable |
| Reachable vulnerabilities | `<actual>` | 0 |
| Bruno requests | `<passed>/<total>` | all pass |
| Bruno tests | `<passed>/<total>` | all pass |
| Bruno assertions | `<passed>/<total>` | all pass |

## References

PR: #<number>
Follow-up: #<number> or None
```

- Use the date in Asia/Bangkok. Set `Status` to `Complete` only when all required checks pass.
- Keep `task check:task`, the PR diff check, and CI in every report. Include `task markdown:check` when Markdown changed. Include other commands and Bruno rows only when they apply.
- Copy measurements from the command output. Use `n/a` for changed-line coverage when the task adds no executable Go lines. If a command stops before it reports a measurement, omit that row. State the failure in Verification.
- For `Failed` or `Not run`, state the cause. Link a follow-up Issue when work remains. Do not include secrets or test account data.

## Final Prove task

The final task in each module plan checks the approved specification through tests and review. Use the Issue template above.

- Title the Issue `Prove <capability> against its specification`. Name the capability so readers can distinguish it from other modules.
- In Description, name the final test scope and any cross-service checks owned by another Issue. Link to that Issue instead of repeating its work.
- In Acceptance criteria, require tests for every success criterion and applicable threat. Do not require a separate evidence file.
- Add acceptance criteria for the module's public, private, failure, and cross-service paths when they apply. Name the outcomes that still need proof.
- For a module with public REST routes, require Bruno smoke tests under `tests/smoke/bruno/` against a running service. Record the command and passing result in the task result comment.
- If a check fails or is missing, record the gap and its follow-up Issue in the task result comment. Keep the task and final plan checkpoint open until the gap is resolved.
- In Verification, list `task check:task`, `task markdown:check`, PR diff inspection, and CI review. Add separate command items for the module's applicable integration, smoke, contract, and generation checks. Report command results in the task result comment.
- Add `Blocked by` relationships for actual blocking Issues. Include `docs/specs/<module-id>.md`, `tasks/<module-id>.md`, and likely test paths in Files likely touched.
- When every criterion passes, mark the specification `Implemented` and the plan `Complete`. Record the final PR in the plan.

## Examples

The [Workspace creation Issue](https://github.com/vasapolrittideah/flowspace-api/issues/48) shows a complete Issue using this template.
