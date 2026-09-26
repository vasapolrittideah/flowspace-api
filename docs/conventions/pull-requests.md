# Pull request conventions

This convention defines how to write and review a pull request (PR) and its suggested squash commit.

## Template

The [PR template](../../.github/pull_request_template.md) contains `Change`, `Reason`, `Risks or limitations`, and `Verification` sections.

| Part | Content and format |
| --- | --- |
| Title | A [commit subject](commit-messages.md#template) that describes the result. |
| Change | State the problem and what happens after the change. |
| Reason | Why the change is needed and any related Issue numbers. |
| Risks or limitations | Material compatibility effects, remaining limits, or follow-up work. |
| Verification | Required checks, measured results, and exact commands. |

## Rules

- Follow the [agent instructions](../../AGENTS.md) for PR creation and merge authority.
- Keep the title, description, and [labels](github-labels.md) consistent with the final work.
- Use only the [commit subject format](commit-messages.md#template), with the same [types](commit-messages.md#types) and [scopes](commit-messages.md#scopes), in the title. Do not include a body or footer. Describe the result, not the branch or changed files.
- End `Reason` with one `Closes #<issue-number>.` line for each Issue the PR completes. Use `Refs #<issue-number>.` when the Issue remains open, such as when a PR updates a spec before implementation. Leave one blank line before the first Issue reference and no blank lines between references. `Refs` does not close the Issue; `Closes` closes it after the PR merges into `main`.
- Use the [PR template](../../.github/pull_request_template.md) as the source for the description. Complete `Change`, `Reason`, and `Verification`. Omit `Risks or limitations` when none apply.
- Before creating or updating a PR description, reread the [Markdown rules](markdown-and-prose.md#markdown). Review every prose section after editing. Keep new text in an existing paragraph or bullet only when it develops the same point. Start a new paragraph for a separate explanation, or use separate bullets for independent points.

### Before requesting review

- Preserve work from other tasks.
- Inspect the staged diff before each commit and the complete PR diff before maintainer review. Exclude unrelated changes, secrets, local environment files, and unwanted build output.
- Run the relevant tests, lint commands, builds, and contract commands. Do not weaken commands or hide failures.
- For a behavior fix, add a focused regression test.
- For a contract or generator change, make sure that regeneration and compatibility succeed and that source files and generated output agree.
- For a documentation-only change, make sure that facts, examples, links, and formatting are correct. Application tests are unnecessary unless executable behavior changes.

### Verification

Use the three parts in the PR template: Checks, Measurements, and Commands. Keep commands in a list below the tables so long commands do not widen them.

#### Checks

- Write one row per required check. Use these names for checks that recur across PRs:

| Check | Command | When |
| --- | --- | --- |
| Repository checks | `task check:task` | Every PR |
| Markdown checks | `task markdown:check` | When Markdown changes |
| Diff whitespace | `git diff origin/main...HEAD --check` | Every PR |

- Give a PR-specific check a short name that describes what it proves, such as `Contract compatibility` or `Milestone issues`. Reuse the same name for the same check in later PRs. Omit checks that do not apply.
- Add a focused check only when it proves something that `task check:task` does not, such as repeated runs of one test.
- Set `Result` to exactly `Passed`, `Failed`, or `Not run`. Do not add punctuation or other text in that cell. If a command fails, fix the failure or mark the PR as needing attention.
- Set `Notes` to `-` when there is nothing to add. State the cause of each unresolved failure and the reason for each check that did not run. Report warnings even when a command passes. Do not paste routine logs or describe resolved attempts. Link relevant output when it helps review.
- Put vulnerabilities in required modules that the code does not appear to call in `Repository checks` Notes. Use this wording: `Govulncheck reported <count> vulnerabilities in required modules that the code does not appear to call.`

#### Measurements

- Record the measured values from `task check:task` in the Measurement table. Use the requirements in [CONSTRAINTS.md](../../CONSTRAINTS.md) for these standard rows:

| Measurement | Constraint |
| --- | --- |
| Project coverage | C5 |
| Changed-line coverage | C4 |
| Reachable vulnerabilities | C7 |

- Copy each requirement and measured value into its own cell. Keep the coverage precision and covered-line count from the command output. Add a PR-specific row when a numeric result has an explicit requirement. Keep other observations in Checks Notes.
- Set `Status` to exactly `Passed` when the value meets the requirement or `Failed` when it does not. Use `Not applicable` when the measurement does not apply. Use `Not measured` when a command did not produce a valid measurement. Set `Value` to `-` when no value exists or `Status` is `Not applicable` or `Not measured`.

#### Commands

- Write one list item for each Check in the same order as the Checks table. Start with the Check name and a colon, then put the exact command in inline code. Include environment variables, flags, redirections, and commands outside Task.
- Keep each command complete. Do not replace arguments with `...`. Before submitting the PR, preview the Commands list on GitHub and make sure each command is complete and readable.
- Use the matching `task ...` command from `Taskfile.yaml` first. If no matching task exists, use another command. For a check that did not run, show the command that still needs to run.

### Suggested squash commit

- Before maintainer review, provide the exact suggested squash message in the chat. Update it if the PR changes, and do not put it in the PR description.
- Use the reviewed PR title as the subject. Follow the [commit message rules](commit-messages.md#rules), including formatting and AI co-authorship.
- If a body is needed, explain important effects and compatibility or migration information. Do not copy detailed verification from the PR.
- Use the GitHub Pull request title and description squash default. The maintainer can shorten the copied description but must keep required context and trailers.

## Examples

This title describes the result instead of the branch:

```text
Branch: fix/duplicate-notifications
Title:  fix(notifications): prevent duplicate delivery when an event is retried
```
