# Commit message conventions

This convention defines the message of each checkpoint commit that people and agents write on a branch. The squash commit that merges a pull request (PR) into `main` has only the PR title, and the PR title follows the subject rules of this convention, as [ADR-0043](../adr/0043-squash-commits-keep-only-the-pull-request-title.md) states. Commits that Renovate creates follow its own configuration, but the title of a Renovate PR follows the subject rules.

The [Repository instructions](../../AGENTS.md) state when to create checkpoint commits and who squash merges a PR.

## Template

A message has a type, a scope, a breaking marker, a description, a body, Issue footers, and co-author trailers.

```text
<type of change>(<scope, if any>)<breaking marker, if any>: <description of the change>

<body, if any>

<Issue footer, if any>

Co-authored-by: <co-author name, if any> <<co-author email>>
```

### Type

- Use the type of the main change.
- If a change only edits Markdown agent instructions, conventions, skills, or commands, use `docs`. If it changes code or configuration for agents, such as a script or a hook, use the type of that change.
- Use one of the [Types](#types).
- Do not use `feat` only because the change adds a file.

### Scope

- Choose the scope with these steps, and stop after the first match:
  1. If the main change adds or changes instrumentation in more than 1 service, use `observability`.
  2. Use the first scope in the [Scopes](#scopes) table whose `Paths` column covers all of the main change. A row can cover a whole path or 1 part of a file, such as a task in `Taskfile.yaml`.
  3. If no row covers all of the main change, omit the scope.
- If a PR adds a service, add its scope and paths to the [Scopes](#scopes) table in the same PR. A new shared package needs no new scope, because `shared` covers every package under root `internal/`.
- Do not combine scope names.

### Breaking marker

- Write the marker as `!` directly before the colon.
- If a contract change breaks callers, add the marker. [ADR-0007](../adr/0007-version-apis-by-compatibility-boundary.md) requires a new version for such a change.
- Do not add the marker for other incompatible changes, such as a renamed environment variable. Explain them in the [Breaking changes](pull-requests.md#breaking-changes) section of the PR description.

### Description

- Start the description with a lowercase verb in the imperative mood, such as `add` or `reject`.
- Do not end the description with a period.
- Name the specific change, such as `reject task updates based on an outdated version`. If you use a general verb, such as `update`, give it a specific object, such as `update Go to 1.25`.
- Do not write a description that names no change, such as `misc` or `fix things`.

### Body

- Put the body after the subject, separated by a blank line.
- Wrap prose lines at 72 characters.
- Separate paragraphs with 1 blank line.
- Start each bullet item with a hyphen and a space, and indent its wrapped lines by 2 spaces.
- Start each numbered item with its number, a period, and a space, and indent its wrapped lines by 3 spaces.
- In a checkpoint commit, write a body only when the subject does not explain the step. Write for a reviewer who reads the PR 1 commit at a time. Explain what the step changes, and the reason or trade-off when the subject does not make it clear. You can name the tests that the step adds, because they are part of the step.
- If a checkpoint subject matches the PR title, apply the checkpoint body rule above. This also applies to the only commit of a PR.
- Do not repeat text or add process history, abandoned methods, hypothetical objections, or unrelated files.
- Do not refer to the commit, the writer, or the time of writing, such as "This commit adds", "I", "we", "now", or "currently". State each change directly. The required first body line in [Commits that revert a commit](#commits-that-revert-a-commit) is an exception.

### Issue footers

- Put the footers after the body, or after the subject when there is no body, separated by a blank line.
- Write each footer as `Refs: #<Issue number>`.
- Do not leave blank lines between Issue footers.
- Do not add a period at the end of an Issue footer.
- Write 1 footer per Issue, and repeat the key on each line.
- Order the footers by ascending Issue number.
- If a checkpoint commit belongs to an Issue, add a `Refs` footer for it.
- Do not use `Closes` in a commit. The [Related issues](pull-requests.md#related-issues) section of the PR description closes each Issue when the PR merges.
- Do not add Issues from [Follow-up tasks](pull-requests.md#follow-up-tasks).

### Co-author trailers

- Put the trailers after a blank line at the end of the message.
- Spell the trailer key as `Co-authored-by`.
- Order the trailers by name in alphabetical order.
- Add 1 trailer for each person or agent, other than the commit author, who contributed to the commit.
- For a person, use their name and the email that GitHub links to their account, such as their `users.noreply.github.com` address. For an agent, use the name and email in the attribution trailer that its harness gives. If the harness gives none, use the identity in [Agent trailers](#agent-trailers).
- Keep the existing trailers when you amend or squash commits. GitHub copies the trailers of the checkpoint commits to the squash commit.
- Do not add a trailer for a person or agent who did not contribute.

## Rules

### Format and content

- Write the whole message as plain text, including text that comes from a PR description. Do not use backticks, Markdown links, or URLs. Name a file by its path, and refer to an Issue or a pull request by its number, such as `#123`.
- Keep the whole subject line at most 72 characters. Do not count the `(#<number>)` suffix and its leading space, which GitHub adds to a squash commit.
- Write the parts in the order shown in the [Template](#template).
- Follow [Conventional Commits 1.0.0](https://www.conventionalcommits.org/en/v1.0.0/).
- Do not split code or trailers. A line that holds one of them can be longer than 72 characters.

### Workflow

- If a message has more than 1 line, write it to a file outside the working tree, and pass the file with `git commit --file <message-file>` in its own command.

### Changes

- Apply a change of this convention only to new commits and to commit messages that a later PR changes. Do not rewrite commits on `main` to follow it.

## Commits that revert a commit

Follow these rules for each commit that reverts an earlier commit. The [What changed](pull-requests.md#what-changed) section of the PR description names the reverted commit for the squash commit.

- Start the body with `This reverts commit <full SHA>.` Then give the reason in a new paragraph.
- Use the scope and the description of the reverted commit.
- Use the `revert` type.
- Do not copy the breaking marker of the reverted commit. Add the marker only when the revert itself breaks callers, as the [Breaking marker](#breaking-marker) rules state.

## Differences from the git-workflow-and-versioning skill

This convention applies where it differs from the [Git Workflow and Versioning](../../.agents/skills/git-workflow-and-versioning/SKILL.md) skill:

- Follow the [Scope](#scope) rules. The skill writes `<type>: <description>` without a scope.
- Follow the [Body](#body) rules for checkpoint commits. The skill explains only why a change was made.
- Follow the [Git workflow](../../AGENTS.md#git-workflow) of the repository instructions. The skill advises against squashing commits.
- Follow the [Type](#type) rules. The skill lists 6 types and has no `perf`, `build`, `ci`, `style`, or `revert` type.

## Reference

### Types

| Type | Purpose |
| --- | --- |
| `feat` | Add functionality |
| `fix` | Correct a bug |
| `docs` | Change documentation only |
| `refactor` | Restructure code without changing behavior |
| `test` | Add or correct tests |
| `perf` | Improve performance |
| `build` | Change build tooling, compilation, or packaging |
| `ci` | Change CI workflows or automated checks |
| `style` | Change formatting only |
| `chore` | Maintain the project when no more specific type fits |
| `revert` | Revert an earlier commit |

### Scopes

| Scope | Area | Paths |
| --- | --- | --- |
| `proto` | Protobuf RPC definitions and public HTTP annotations | `contracts/proto/` |
| `events` | Published Protobuf event schemas | `contracts/events/` |
| `codegen` | Code-generation configuration and tooling | `buf.yaml`, `buf.gen.yaml`, `buf.lock`, `sqlc.yaml`, `gen/`, and the `buf` and `sqlc` tasks in `Taskfile.yaml` |
| `adr` | Architecture decision records | `docs/adr/` |
| `agents` | Agent instructions, conventions, skills, commands, and configuration | `AGENTS.md`, `CLAUDE.md`, `docs/conventions/`, `.agents/`, `.claude/`, `.codex/`, `skills-lock.json` |
| `identity` | Accounts, authentication, email verification, and sessions | The service paths for `identity` |
| `workspace` | Workspaces, memberships, invitations, roles, and authorization | The service paths for `workspace` |
| `work` | Projects, tasks, assignments, status transitions, comments, activity history, and the event outbox | The service paths for `work` |
| `notifications` | In-app notification inbox, read state, and event deduplication | The service paths for `notifications` |
| `observability` | Telemetry stack, dashboards, alert rules, and observability specifications, plans, and runbooks | `docs/specs/observability-*`, `tasks/observability-*`, `docs/runbooks/`, and the configuration of the telemetry stack under `deploy/` |
| `shared` | Shared technical Go packages | `internal/` |
| `infra` | Infrastructure and deployment configuration | `deploy/`, `Tiltfile`, and the `cluster:*` tasks in `Taskfile.yaml` |
| `deps` | Dependency updates | `go.mod`, `go.sum`, `renovate.json` |

### Agent trailers

| Agent | Trailer |
| --- | --- |
| Codex | `Co-authored-by: Codex <noreply@openai.com>` |
| Claude Code | `Co-authored-by: Claude <noreply@anthropic.com>` |

## Examples

### Subjects

This subject uses `feat` for a new workspace feature:

```text
feat(workspace): allow owners to invite workspace members
```

This subject uses `fix` for a bug fix in the work service:

```text
fix(work): reject task updates based on an outdated version
```

This subject uses `test` for tests of an identity specification:

```text
test(identity): prove provider login against its specification
```

This subject uses the `observability` scope for an observability specification:

```text
docs(observability): approve observability logs and traces spec
```

This subject uses the `adr` scope for an architecture decision record:

```text
docs(adr): explain the choice of squash merging
```

This subject uses `fix` for a bug fix in agent code:

```text
fix(agents): preserve multiline PR descriptions
```

This subject uses the `codegen` scope for code-generation configuration:

```text
build(codegen): configure Buf to generate ConnectRPC clients
```

This subject has no scope, because no row of the [Scopes](#scopes) table covers the CI workflow:

```text
ci: add pull request title validation
```

This subject has a breaking marker for a change that breaks callers:

```text
feat(workspace)!: require a role when inviting members
```

This subject reverts an earlier commit with its scope and description:

```text
revert(agents): clarify convention headings
```

### Checkpoint commits

This checkpoint names the tests of its step and uses `Refs` to keep its Issue open until the PR merges:

```text
feat(identity): store trace context with outbox events

Migration 00009 adds nullable traceparent and tracestate columns to
identity_outbox_events, and the outbox insert stores the context of
the current span. TestOutboxStoresTraceContext checks the stored
values in a PostgreSQL container.

Refs: #304

Co-authored-by: Claude Opus 5.5 <noreply@anthropic.com>
```

This documentation checkpoint shows a complete message with Codex attribution:

```text
docs(agents): clarify convention headings

Co-authored-by: Codex <noreply@openai.com>
```

### Revert commits

This revert commit keeps the scope and description of the reverted commit and names the reverted commit and the reason in its body:

```text
revert(agents): clarify convention headings

This reverts commit 2f77dde4c1b9a6e3d5f8a0b7c2e4d6f8a1b3c5e7.

The new headings broke links from other convention files.

Co-authored-by: Claude Opus 5.5 <noreply@anthropic.com>
```
