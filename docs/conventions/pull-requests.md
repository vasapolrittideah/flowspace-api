# Pull request conventions

This convention defines how to write and review a pull request (PR) and its suggested squash commit.

## Template

The [PR template](../../.github/pull_request_template.md) contains `Summary`, `Risks or limitations`, and `Notes` sections.

| Part | Content and format |
| --- | --- |
| Title | A [commit subject](commit-messages.md#template) that describes the result. |
| Summary | State the problem, what changes, why it is needed, and any related Issue numbers. |
| Risks or limitations | Material compatibility effects, remaining limits, or follow-up work. |
| Notes | Useful context about the PR that does not fit the other sections. |

## Rules

- Follow the [agent instructions](../../AGENTS.md) for PR creation and merge authority.
- Keep the title, description, and [labels](github-labels.md) consistent with the final work.
- Use only the [commit subject format](commit-messages.md#template), with the same [types](commit-messages.md#types) and [scopes](commit-messages.md#scopes), in the title. Do not include a body or footer. Describe the result, not the branch or changed files.
- End `Summary` with one `Closes #<issue-number>.` line for each Issue the PR completes. Use `Refs #<issue-number>.` when the Issue remains open, such as when a PR updates a spec before implementation. Leave one blank line before the first Issue reference and no blank lines between references. `Refs` does not close the Issue; `Closes` closes it after the PR merges into `main`.
- Use the [PR template](../../.github/pull_request_template.md) as the source for the description. Complete `Summary`, `Risks or limitations`, and `Notes`. Write `n/a` in either of the last two sections when there is nothing to report.
- Before creating or updating a PR description, reread the [Markdown rules](markdown-and-prose.md#markdown). Review every prose section after editing. Keep new text in an existing paragraph or bullet only when it develops the same point. Start a new paragraph for a separate explanation, or use separate bullets for independent points.

### Before requesting review

- Preserve work from other tasks.
- Inspect the staged diff before each commit and the complete PR diff before maintainer review. Exclude unrelated changes, secrets, local environment files, and unwanted build output.
- Run the relevant tests, lint commands, builds, and contract commands. Do not weaken commands or hide failures.
- For a behavior fix, add a focused regression test.
- For a contract or generator change, make sure that regeneration and compatibility succeed and that source files and generated output agree.
- For a documentation-only change, make sure that facts, examples, links, and formatting are correct. Application tests are unnecessary unless executable behavior changes.

### Checks

Run `task check:task` before requesting review. If Markdown changes, run `task markdown:check`. Check the PR diff with `git diff origin/main...HEAD --check`. Use a focused check when it proves behavior that these commands do not cover.

Review the [CI checks](../../.github/workflows/ci.yml) on the PR. CI reports the required results and measurements, including coverage and reachable vulnerabilities. Do not copy those results into the PR description.

### Notes

- Write useful facts about this PR that do not fit the other sections. Write paragraphs for connected points and bullets for independent points. A note does not need to start with a Check name.
- State the cause of each unresolved failure and the reason for each local check that did not run. Report warnings even when a command passes. Do not paste routine logs or describe resolved attempts. Link relevant output when it helps review.
- For vulnerabilities in required modules that the code does not appear to call, use this wording: `Govulncheck reported <count> vulnerabilities in required modules that the code does not appear to call.`

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
