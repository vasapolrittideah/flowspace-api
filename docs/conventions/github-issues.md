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
- Include `task check:task`, `task git:diff:check`, and CI review in every Verification list. Include `task markdown:check` when Markdown changes. Add commands that check the task's behavior.
- Keep inspection of a command's output in the same Verification item when they form one check. Put separate manual checks in separate items.
- Add one native GitHub `Blocked by` relationship for each blocking Issue. Use these relationships as the dependency list.
- For generated output, list only its folder. Do not list generated file names.

## Task result comment

After verification, check each completed item in the Issue body. Leave failed and unrun items unchecked. Post the run result as a comment. Keep measurements and run details out of the Issue body.

```markdown
## Result

Date: YYYY-MM-DD
Status: Complete | Blocked
Outcome: <State the result in one sentence.>
Environment: <State where the local checks ran and name any test services, or write CI only.>
CI: <Passed | Failed | Not run>. [Run <run-id>](https://github.com/vasapolrittideah/flowspace-api/actions/runs/<run-id>).

## Measurements

| Measure | Result | Required |
| --- | --- | --- |
| Project coverage | <actual>% | at least 25.0% |
| Changed-line coverage | <actual>% or n/a | at least 80% when applicable |
| Reachable vulnerabilities | <actual> | 0 |
| Bruno requests | <passed>/<total> | all pass |
| Bruno tests | <passed>/<total> | all pass |
| Bruno assertions | <passed>/<total> | all pass |

## Gaps

None

## References

PR: #<number>
Follow-up: #<number> or None
```

- Set `Status` to `Complete` only when all required checks pass.
- Do not repeat the completed Verification checklist in the comment. Keep `None` under `Gaps` when every required check passes. For each failed or unrun item, leave its checkbox clear. Replace `None` with a bullet that names the item, cause, and next action.
- Prefer CI measurements. Use local output if CI did not produce a measurement. Write values in the Result column as plain text. Include Bruno rows only when Bruno applies. Use n/a for changed-line coverage when the task adds no executable Go lines. If a command stops before it reports a measurement, omit that row and explain the failure under `Gaps`.
- Keep the CI run link in every report. If CI did not run, replace the link with the reason. Link a follow-up Issue when work remains. Do not include secrets or test account data.

## Final Prove task

The final task in each module plan checks the approved specification through tests and review. Use the Issue template above.

- Title the Issue `Prove <capability> against its specification`. Name the capability so readers can distinguish it from other modules.
- In Description, name the final test scope and any cross-service checks owned by another Issue. Link to that Issue instead of repeating its work.
- In Acceptance criteria, require tests for every success criterion and applicable threat.
- Add acceptance criteria for the module's public, private, failure, and cross-service paths when they apply. Name the outcomes that still need proof.
- For a module with public REST routes, require Bruno smoke tests under `tests/smoke/bruno/` against a running service. Run `task smoke:bruno` and record the result in the task result comment.
- If a check fails or is missing, record the gap and its follow-up Issue in the task result comment. Keep the task and final plan checkpoint open until the gap is resolved.
- Add separate Verification items for the module's applicable integration, smoke, contract, and generation checks. Check completed items in the Issue body and report only gaps in the task result comment.
- Add `Blocked by` relationships for actual blocking Issues. Include `docs/specs/<module-id>.md`, `tasks/<module-id>.md`, and likely test paths in Files likely touched.
- When every criterion passes, mark the specification `Implemented` and the plan `Complete`. Record the final PR in the plan.

## Examples

The [Workspace creation Issue](https://github.com/vasapolrittideah/flowspace-api/issues/48) shows a complete Issue using this template.
