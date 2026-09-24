# Pull request conventions

The [agent instructions](../../AGENTS.md) define who can open and merge pull requests (PRs). Use one PR for one reviewable change.

## Description

Use the [PR template](../../.github/pull_request_template.md). Complete `Change`, `Reason`, and `Verification`.

- `Change` states the problem and what happens after the change.
- `Reason` states why the change is needed. Add `Closes #<issue-number>` for each Issue that the PR completes. GitHub closes those Issues after the PR merges into `main`.
- `Verification` records the commands and results as described [below](#verification).
- `Risks or limitations` names material compatibility effects, remaining limits, or follow-up work. Omit this section when none apply.

Keep each paragraph and list item on one physical line. Keep the title, description, and [labels](github-labels.md) consistent with the final work.

## Title

Use only the [commit subject format](commit-messages.md#subject), with the same [types](commit-messages.md#types) and [scopes](commit-messages.md#scopes). Do not include a body or footer in the title. Put details in the description.

Describe the result, not the branch or changed files:

```text
Branch: fix/duplicate-notifications
Title:  fix(notifications): prevent duplicate delivery when an event is retried
```

## Verification

Before review, complete these actions:

- Inspect the staged diff before each commit. Inspect the complete PR diff before maintainer review.
- Exclude unrelated changes, secrets, local environment files, and unwanted build output.
- Run the relevant tests, lint commands, builds, and contract commands.
- For a behavior fix, add a focused regression test.
- For a contract or generator change, make sure that regeneration and compatibility succeed. Make sure that source files and generated output agree.
- For a documentation-only change, make sure that facts, examples, links, and formatting are correct. Application tests are unnecessary unless executable behavior changes.
- Do not weaken commands, hide failures, or discard work from another task.

Write one row per required check in the PR Verification table:

- Use the matching `task ...` command from `Taskfile.yaml` first. If no matching task exists, use another command.
- Record each required command and its exact result, including commands outside Task. Omit checks that do not apply.
- If a required check did not run, write `Not run.` and give the reason. Do not report it as passing.
- If a command fails, fix the failure or mark the PR as needing attention.
- Use a short check name in `Check` and the exact command in `Command`.
- Start `Result` with `Passed.`, `Failed.`, or `Not run.`. Report warnings and unresolved failures even when a command exits successfully.
- Add a short explanation only when it helps review, such as coverage values or a failure cause. Do not paste routine logs or describe resolved attempts. Link to relevant output when a result needs more context.

## Suggested squash commit

Before maintainer review, provide the exact suggested squash message in the chat. Update it if the PR changes. Do not put it in the PR description.

- Use the reviewed PR title as the subject.
- Follow the [commit message](commit-messages.md), [formatting](commit-messages.md#formatting), and [AI co-authorship](commit-messages.md#ai-co-authorship) rules.
- If a body is needed, explain important effects and compatibility or migration information. Do not copy detailed verification from the PR.
- Use the GitHub Pull request title and description squash default.

The maintainer can shorten the copied description but must keep required context and trailers.
