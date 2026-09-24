# Pull request conventions

This convention defines how to write and review a pull request (PR) and its suggested squash commit.

## Template

The [PR template](../../.github/pull_request_template.md) contains `Change`, `Reason`, `Risks or limitations`, and `Verification` sections.

| Part | Content and format |
| --- | --- |
| Title | A [commit subject](commit-messages.md#template) that describes the result. |
| Change | State the problem and what happens after the change. |
| Reason | Why the change is needed and any completed Issue numbers. |
| Risks or limitations | Material compatibility effects, remaining limits, or follow-up work. |
| Verification | Each required check, its exact command, and its result. |

## Rules

- Follow the [agent instructions](../../AGENTS.md) for PR creation and merge authority.
- Follow this convention when creating or updating a PR or preparing its squash message.
- Keep the title, description, and [labels](github-labels.md) consistent with the final work.
- Use only the [commit subject format](commit-messages.md#template), with the same [types](commit-messages.md#types) and [scopes](commit-messages.md#scopes), in the title. Do not include a body or footer. Describe the result, not the branch or changed files.
- Add `Closes #<issue-number>` to `Reason` for each Issue that the PR completes. GitHub closes those Issues after the PR merges into `main`.
- Use the [PR template](../../.github/pull_request_template.md) as the source for the description. Complete `Change`, `Reason`, and `Verification`. Omit `Risks or limitations` when none apply.
- Follow the [Markdown rules](markdown-and-prose.md#markdown) in the prose sections. Use a paragraph for one connected point and bullets for multiple independent points. Keep each paragraph and list item on one physical line.

### Before requesting review

- Preserve work from other tasks.
- Inspect the staged diff before each commit and the complete PR diff before maintainer review. Exclude unrelated changes, secrets, local environment files, and unwanted build output.
- Run the relevant tests, lint commands, builds, and contract commands. Do not weaken commands or hide failures.
- For a behavior fix, add a focused regression test.
- For a contract or generator change, make sure that regeneration and compatibility succeed and that source files and generated output agree.
- For a documentation-only change, make sure that facts, examples, links, and formatting are correct. Application tests are unnecessary unless executable behavior changes.

### Verification

- If a command fails, fix the failure or mark the PR as needing attention. Report warnings and unresolved failures even when a command exits successfully.
- If a required check did not run, write `Not run.` and give the reason. Do not report it as passing.
- Write one row per required check in the PR Verification table. Omit checks that do not apply.
- Use the matching `task ...` command from `Taskfile.yaml` first. If no matching task exists, use another command. Record each required command and its exact result, including commands outside Task.
- Use a short check name in `Check` and the exact command in `Command`. Start `Result` with `Passed.`, `Failed.`, or `Not run.`.
- Add a short explanation only when it helps review, such as coverage values or a failure cause. Do not paste routine logs or describe resolved attempts. Link relevant output when a result needs more context.

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

The [project conventions PR](https://github.com/vasapolrittideah/flowspace-api/pull/121) shows the PR template with a completed Verification table.
