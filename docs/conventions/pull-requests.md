# Pull request conventions

This convention defines how to write and review a pull request (PR) and its suggested squash commit.

## Template

A PR has a title and the description sections of the [PR template](../../.github/pull_request_template.md).

```markdown
## What changed

<what changed and the result>

## Why

<problem and the reason for the change>

## Related issues

<Issues that the PR completes or refers to>

## Risks or limitations

<material compatibility effects, unresolved failures, checks that did not run, or remaining limits>

## Follow-up tasks

<open Issues for later work>
```

### Title

- Use only the [commit subject format](commit-messages.md#template), with the same [types](commit-messages.md#types) and [scopes](commit-messages.md#scopes).
- Describe the result, not the branch or changed files.
- Do not include a body or footer.

### What changed

- Use a paragraph for one point and bullets for several independent points.

### Why

- Use a paragraph for one point and bullets for several independent points.
- Link the relevant decisions.

### Related issues

- Write one line per Issue, and repeat the keyword on each line, because GitHub ignores an Issue that follows a comma. Use the line order and spacing of the [Issue footers](commit-messages.md#issue-footers), but keep the period at the end of each line.
- Write `Closes #<issue-number>.` for each Issue that the PR completes. GitHub closes the Issue after the PR merges into `main`.
- Write `Refs #<issue-number>.` for each Issue that stays open, such as when a PR updates a spec before implementation.
- If no Issue is related, write `n/a`.
- Do not use `Fixes`, although GitHub accepts it. Use `Closes` for consistency.
- Do not repeat a follow-up Issue.

### Risks or limitations

- Use a paragraph for one point and bullets for several independent points.
- State the cause of each unresolved failure and the reason for each local check that did not run. Include material warnings and security risks.
- When a limitation has follow-up work, describe the limitation and refer to `Follow-up tasks` instead of repeating the Issue.
- Link relevant output when it helps review.
- If there is nothing to report, write `n/a`.
- Do not copy module-only Govulncheck counts, paste routine logs, or describe resolved attempts.

### Follow-up tasks

- Write each follow-up Issue in the [follow-up task format](github-issues.md#follow-up-task-format), even when there is only one.
- If there is no follow-up Issue, write `n/a`.

## Rules

- Follow the [agent instructions](../../AGENTS.md) for PR creation and merge authority.
- Follow the [label rules](github-labels.md), which also keep the title and description consistent with the final work.
- Use the [PR template](../../.github/pull_request_template.md) as the source for the description. Complete every section.
- Before creating or updating a PR description, reread the [Markdown rules](markdown-and-prose.md#markdown). Review every prose section after editing. Keep new text in an existing paragraph or bullet only when it develops the same point. Start a new paragraph for a separate explanation, or use separate bullets for independent points.

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

### Suggested squash commit

- Before you write each suggested squash message, read the [commit message convention](commit-messages.md) again from `main`, because it can change while a PR is open.
- Before maintainer review, provide the exact suggested squash message in the chat. Update it if the PR changes, and do not put it in the PR description.
- Use the reviewed PR title as the subject. Follow the [commit message convention](commit-messages.md), including formatting and co-author trailers.
- If the [body rules](commit-messages.md#body) require a body, write it from the final description as those rules state. Otherwise, omit the body.
- Copy `Related issues` into the [Issue footers](commit-messages.md#issue-footers) in the same order.
- Use the GitHub Pull request title and description squash default. The maintainer can shorten the copied description but must keep required context and trailers.

## Examples

This title describes the result instead of the branch:

```text
Branch: fix/duplicate-notifications
Title:  fix(notifications): prevent duplicate delivery when an event is retried
```
