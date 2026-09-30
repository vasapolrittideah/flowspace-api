# Pull request conventions

This convention defines how to write and review a pull request (PR) and its suggested squash commit.

## Template

The [PR template](../../.github/pull_request_template.md) contains `What changed`, `Why`, `Related issues`, `Risks or limitations`, and `Follow-up tasks` sections.

- Title: A [commit subject](commit-messages.md#template) that describes the result.
- What changed: State what changed and the result.
- Why: State the problem and why the change is needed.
- Related issues: Add one reference per related Issue, or `n/a` when there is none.
- Risks or limitations: Material compatibility effects, unresolved failures, checks that did not run, or remaining limits.
- Follow-up tasks: One bullet per open Issue for later work, or `n/a` when there is none.

## Rules

- Follow the [agent instructions](../../AGENTS.md) for PR creation and merge authority.
- Keep the title, description, and [labels](github-labels.md) consistent with the final work.
- Use only the [commit subject format](commit-messages.md#template), with the same [types](commit-messages.md#types) and [scopes](commit-messages.md#scopes), in the title. Do not include a body or footer. Describe the result, not the branch or changed files.
- In `Related issues`, write one `Closes #<issue-number>.` line for each Issue the PR completes. Use `Refs #<issue-number>.` when an Issue remains open, such as when a PR updates a spec before implementation. Write one line per Issue, and repeat the keyword on each line, because GitHub ignores an Issue that follows a comma. Put `Closes` lines before `Refs` lines, and order each group by ascending Issue number. Do not leave blank lines between references. `Refs` does not close the Issue. `Closes` closes it after the PR merges into `main`. GitHub also accepts `Fixes`, but use `Closes` for consistency.
- Use the [PR template](../../.github/pull_request_template.md) as the source for the description. Complete every section. Write `n/a` in `Related issues`, `Risks or limitations`, or `Follow-up tasks` when there is nothing to report.
- Before creating or updating a PR description, reread the [Markdown rules](markdown-and-prose.md#markdown). Review every prose section after editing. Keep new text in an existing paragraph or bullet only when it develops the same point. Start a new paragraph for a separate explanation, or use separate bullets for independent points. Do not use a bullet when a section has only one point, except in `Follow-up tasks`.

### Follow-up tasks

- Write one bullet per Issue, even when there is only one: `- #<issue-number>: <remaining work in one sentence>.` GitHub shows only the number in the description, so the sentence names the work.
- List only open Issues. Create an Issue for later work before you list it, or leave the work out.
- Do not put `Closes`, `Fixes`, or `Resolves` before a follow-up Issue number. GitHub would close the Issue when the PR merges.
- Do not repeat a follow-up Issue in `Related issues`. When a limitation has follow-up work, describe the limitation in `Risks or limitations` and refer to `Follow-up tasks` instead of repeating the Issue.

## Review readiness

### Before requesting review

- Preserve work from other tasks.
- Inspect the staged diff before each commit and the complete PR diff before maintainer review. Exclude unrelated changes, secrets, local environment files, and unwanted build output.
- Run the relevant tests, lint commands, builds, and contract commands. Do not weaken commands or hide failures.
- For a behavior fix, add a focused regression test.
- For a contract or generator change, make sure that regeneration and compatibility succeed and that source files and generated output agree.
- For a documentation-only change, make sure that facts, examples, links, and formatting are correct. Application tests are unnecessary unless executable behavior changes.

### Checks

Run `task check:task` before requesting review. If Markdown changes, run `task markdown:check`. Check the PR diff with `task git:diff:check`. Use a focused check when it proves behavior that these commands do not cover.

Review the [CI checks](../../.github/workflows/ci.yml) on the PR. CI reports the required results and measurements, including coverage and reachable vulnerabilities. Do not copy those results into the PR description.

In `Risks or limitations`, state the cause of each unresolved failure and the reason for each local check that did not run. Include material warnings and security risks. Do not copy module-only Govulncheck counts, paste routine logs, or describe resolved attempts. Link relevant output when it helps review.

### Suggested squash commit

- Before maintainer review, provide the exact suggested squash message in the chat. Update it if the PR changes, and do not put it in the PR description.
- Use the reviewed PR title as the subject. Follow the [commit message rules](commit-messages.md#rules), including formatting and AI co-authorship.
- If a body is needed, explain important effects and compatibility or migration information. Do not copy detailed verification from the PR.
- Copy `Related issues` into the [Issue footers](commit-messages.md#message-format) in the same order.
- Use the GitHub Pull request title and description squash default. The maintainer can shorten the copied description but must keep required context and trailers.

## Examples

This title describes the result instead of the branch:

```text
Branch: fix/duplicate-notifications
Title:  fix(notifications): prevent duplicate delivery when an event is retried
```
