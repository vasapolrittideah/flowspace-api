# Pull request conventions

This convention defines how to write a pull request (PR), how to prepare it for review, and how to write its squash message. The [agent instructions](../../AGENTS.md) define a PR and who merges it. A squash message is the commit message that an agent writes for the squash commit of a PR. The maintainer pastes it into GitHub when they squash merge the PR. A PR is ready when the local checks that the Workflow rules name and CI pass, and the title, description, and labels match the final work. A review comment is a comment or a review on the PR in GitHub. Feedback in the chat is not a review comment. Work for later is work that the PR does not do but shows to be needed, such as a gap that the PR finds or a step that its goal still needs. A risk or effect is material when it can change the decision to merge or needs an action after the merge.

## Template

A PR has a title, What changed, Why, Related issues, Risks or limitations, and Follow-up tasks.

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

<work that the PR leaves for later>
```

### Title

- Use only the [subject line format](commit-messages.md#template) of a commit message, with the same [types](commit-messages.md#types) and [scopes](commit-messages.md#scopes).
- Describe the result, not the branch or the changed files.
- Do not include a body or footer.

### What changed

- State the result of the merged PR first. Then state each change that a reviewer must know to review the PR.
- Do not list every changed file, because the diff shows them.

### Why

- State the problem that the PR solves and the reason for the chosen change.
- Link each decision that the change depends on, such as an ADR or a specification.

### Related issues

- Write one line per Issue, and repeat the keyword on each line, because GitHub ignores an Issue that follows a comma. Use the line order and spacing of the [Issue footers](commit-messages.md#issue-footers), but keep the period at the end of each line.
- Write `Closes #<issue-number>.` for each Issue that the PR completes. GitHub closes the Issue after the PR merges into `main`.
- Write `Refs #<issue-number>.` for each Issue that stays open, such as when a PR updates a spec before implementation.
- If no Issue is related, write `n/a`.
- Do not use `Fixes`, although GitHub accepts it. Use `Closes` for consistency.
- Do not repeat a follow-up Issue.

### Risks or limitations

- Write each material compatibility effect, unresolved failure, local check that did not run, security risk, and remaining limit.
- For each unresolved failure, state the cause and link the CI run or the output that shows it. For each local check that did not run, state the reason.
- When a limitation has follow-up work, describe the limitation and refer to `Follow-up tasks` instead of repeating the work.
- If there is nothing to report, write `n/a`.
- Do not copy the Govulncheck counts of vulnerabilities in required modules that the code does not call. Do not paste routine logs or describe failed attempts that the PR resolved.

### Follow-up tasks

- Write one bullet for each piece of work that the PR leaves for later, even when there is only one.
- If the work has an open Issue, write the bullet in the [follow-up task format](github-issues.md#follow-up-task-format). If the work has no open Issue, write the remaining work as one sentence without a number, such as `- Rename the hexagonal convention file.` This rule replaces the rule of the follow-up task format that lists only open Issues.
- Put the bullets with an Issue first, in ascending Issue number. Then put the bullets without an Issue.
- Before you create an Issue for the work, ask the maintainer. If the maintainer approves, create the Issue as the [Issue convention](github-issues.md) states. If the maintainer does not approve it, or the session has no chat, write the work without a number.
- If the PR leaves no work for later, write `n/a`.

## Rules

### Sections and wording

- Start the description from the [PR template](../../.github/pull_request_template.md), and complete every section.
- Delete the HTML comments of the template. If a section has content, replace its `n/a`.
- If the agent harness gives an attribution line for PR descriptions, put it at the end of the description, after `Follow-up tasks`.

### Workflow

- Follow the [agent instructions](../../AGENTS.md) for the branch, the PR, and merge authority.
- Keep changes that belong to another task out of your commits, and keep them in the working tree. Inspect the staged diff before each commit, and exclude unrelated changes, secrets, local environment files, and unwanted build output.
- For a behavior fix, add a focused regression test.
- For a contract or generator change, run `task buf -- lint`, `task buf -- breaking`, and `task buf -- generate` for Protobuf, or `task sqlc -- generate` for SQL. Commit the generated output with the source change. The `Contract checks` CI job checks the same results again.
- For a documentation-only change, make sure that the facts, examples, links, and formatting are correct. Application tests are unnecessary unless executable behavior changes.
- Before you open or update a PR, run `task check:task`, `task git:diff:check`, and each command in the `Verification` list of each related Issue. If Markdown changes, also run `task markdown:check`. Add a focused check when it proves behavior that these commands do not cover.
- Before you open or update a PR, inspect the complete PR diff with the same exclusions as for a commit.
- Open the PR as a normal PR, not as a draft.
- After each push, wait for the [CI checks](../../.github/workflows/ci.yml) to finish. If the job log shows that a failure comes from the runner, the network, or an external service, such as a registry timeout, rerun the failed job once. If it fails again, state the cause and link the run in `Risks or limitations`. Fix every other failure in the PR.
- Tell the maintainer that the PR is ready only after it meets every condition of a ready PR. If the PR cannot become ready, tell the maintainer which condition fails and why. Tell the maintainer in the chat, or in a PR comment if the session has no chat.
- When the work changes, update the title, the description, and the labels before you tell the maintainer that the PR is ready again.
- Before a review comment exists, you can amend or rebase commits and push them with `git push --force-with-lease`. After a review comment exists, add new commits. If the PR has merge conflicts or needs a change that is on `main`, merge `main` into the branch.
- Before you write each squash message, read the [commit message convention](commit-messages.md) again from `main`, because it can change while a PR is open.
- Write the squash message as the commit message convention states for a squash commit. Use the current PR title as the subject.
- Give the exact squash message in the chat before you tell the maintainer that the PR is ready. If the session has no chat, post the squash message as a PR comment. When the PR changes, give the updated squash message.
- Do not weaken a command or hide a failure.
- Do not copy CI results or measurements, such as coverage or reachable vulnerabilities, into the description.
- Do not put the squash message in the description.
- Do not force-push after a review comment exists.

### Links and records

- Follow the [label rules](github-labels.md).
- Do not set an assignee or a reviewer, and do not add the PR to a GitHub Project.

### Changes

- Apply a change of this convention to new PRs, and to open PRs when their description changes. Do not edit merged PRs to follow it.

## Examples

This title describes the result instead of the branch:

```text
Branch: fix/duplicate-notifications
Title:  fix(notifications): prevent duplicate delivery when an event is retried
```
