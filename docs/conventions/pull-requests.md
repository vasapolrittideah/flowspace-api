# Pull request conventions

## Overview

A pull request (PR) proposes one reviewable change. The [agent instructions](../../AGENTS.md) define who can open and merge it.

## When to Follow

Follow this convention when writing or updating a PR title and description, checking the PR before review, or suggesting a squash commit.

### Before review

Inspect the staged diff before each commit and the complete PR diff before maintainer review. Exclude unrelated changes, secrets, local environment files, and unwanted build output. Run the relevant tests, lint commands, builds, and contract commands. Do not weaken commands, hide failures, or discard work from another task.

For a behavior fix, add a focused regression test. For a contract or generator change, make sure that regeneration and compatibility succeed and that source files and generated output agree. For a documentation-only change, make sure that facts, examples, links, and formatting are correct. Application tests are unnecessary unless executable behavior changes.

### Suggested squash commit

Before maintainer review, provide the exact suggested squash message in the chat. Update the message if the PR changes. Do not put it in the PR description. Use the reviewed PR title as the subject and follow the [commit message](commit-messages.md), [formatting](commit-messages.md#formatting), and [AI co-authorship](commit-messages.md#ai-co-authorship) rules.

If a body is needed, explain important effects and compatibility or migration information. Do not copy detailed verification from the PR. Use the GitHub Pull request title and description squash default. The maintainer can shorten the copied description but must keep required context and trailers.

## Template

Use the [PR template](../../.github/pull_request_template.md) as the source for the description. Complete `Change`, `Reason`, and `Verification`. Omit `Risks or limitations` when none apply. Keep each paragraph and list item on one physical line. Keep the title, description, and [labels](github-labels.md) consistent with the final work.

| Part | How to write it |
| --- | --- |
| Title | Use only the [commit subject format](commit-messages.md#subject), with the same [types](commit-messages.md#types) and [scopes](commit-messages.md#scopes). Do not include a body or footer in the title. Describe the result, not the branch or changed files. |
| Change | State the problem and what happens after the change. |
| Reason | State why the change is needed. Add `Closes #<issue-number>` for each Issue that the PR completes. GitHub closes those Issues after the PR merges into `main`. |
| Risks or limitations | Name material compatibility effects, remaining limits, or follow-up work. Omit this section when none apply. |

### Verification

Write one row per required check in the PR Verification table. Use the matching `task ...` command from `Taskfile.yaml` first. If no matching task exists, use another command. Record each required command and its exact result, including commands outside Task. Omit checks that do not apply.

Use a short check name in `Check` and the exact command in `Command`. Start `Result` with `Passed.`, `Failed.`, or `Not run.`. If a required check did not run, write `Not run.` and give the reason. Do not report it as passing. If a command fails, fix the failure or mark the PR as needing attention. Report warnings and unresolved failures even when a command exits successfully. Add a short explanation only when it helps review, such as coverage values or a failure cause. Do not paste routine logs or describe resolved attempts. Link relevant output when a result needs more context.

## Examples

This title describes the result instead of the branch:

```text
Branch: fix/duplicate-notifications
Title:  fix(notifications): prevent duplicate delivery when an event is retried
```

The [project conventions PR](https://github.com/vasapolrittideah/flowspace-api/pull/121) shows the PR template with a completed Verification table.
