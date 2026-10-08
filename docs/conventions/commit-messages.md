# Commit message conventions

This convention defines the message of each commit that people and agents write: the checkpoint commits on a branch and the squash commit that merges a pull request (PR) into `main`. Commits that Renovate creates follow its own configuration, but the squash commit of a Renovate PR follows this convention.

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
- Do not add the marker for other incompatible changes, such as a renamed environment variable. Explain them in the body.

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
- If the subject has a breaking marker, explain the incompatibility and the changes that callers must make.
- In a checkpoint commit, write a body only when the subject does not explain the step. Write for a reviewer who reads the PR 1 commit at a time. Explain what the step changes, and the reason or trade-off when the subject does not make it clear. You can name the tests that the step adds, because they are part of the step.
- If a checkpoint subject matches the PR title, apply the checkpoint body rule above. This also applies to the only commit of a PR. Do not omit the body only because the squash commit can have no body. The squash body leaves out text that only reviewers need, such as the tests that the step adds.
- In a squash commit, write for a later reader of the `main` history. Choose the body with these steps, and stop after the first match:
  1. If the main change only adds a document or changes its status, omit the body. Examples include a specification, plan, or ADR. The document on `main` holds its content.
  2. If the main change edits an existing rule in a document, write only the reason or trade-off from the [Why](pull-requests.md#why) section of the PR. Include only what the changed files do not state. If the files state the reason, omit the body.
  3. Describe the lasting effects on `main` that the subject and the diff do not state. Take changed behavior and compatibility or migration effects from [What changed](pull-requests.md#what-changed), and important decisions or trade-offs from [Why](pull-requests.md#why). If there are none, omit the body.
- In a squash commit, rewrite the points that the steps above select in plain text for a reader of `main`.
- In a squash commit, take the points from the final version of the PR description.
- In a squash commit, do not copy sentences from the PR description.
- In a squash commit, make sure that the final diff supports each sentence.
- Do not repeat text or add process history, abandoned methods, hypothetical objections, or unrelated files.
- Do not refer to the commit, the writer, or the time of writing, such as "This commit adds", "I", "we", "now", or "currently". State each change directly. The required first body line in [Commits that revert a commit](#commits-that-revert-a-commit) is an exception.
- In a squash commit, do not include verification details, lists of added tests, or specification or plan status changes.
- In a squash commit, do not include text that only reviewers need, such as merge order, review notes, checks that did not run, or follow-up tasks.

### Issue footers

- Put the footers after the body, or after the subject when there is no body, separated by a blank line.
- Write each footer as `Closes: #<Issue number>` or `Refs: #<Issue number>`.
- Do not leave blank lines between Issue footers.
- Do not add a period at the end of an Issue footer.
- Write 1 footer per Issue, and repeat the key on each line.
- Put `Closes` footers before `Refs` footers, and order each group by ascending Issue number.
- Use `Closes` only in a squash commit. GitHub closes the Issue when the commit reaches `main`.
- If a checkpoint commit belongs to an Issue, add a `Refs` footer for it.
- In a squash commit, copy the PR's [Related issues](pull-requests.md#related-issues) as footers: `Closes` for each Issue it completes and `Refs` for each Issue that stays open.
- In the squash commit of a PR whose main change is a module plan, omit the `Refs` footers, because the plan links each of its Issues. Keep each `Closes` footer.
- Do not add Issues from [Follow-up tasks](pull-requests.md#follow-up-tasks).

### Co-author trailers

- Put the trailers after a blank line at the end of the message.
- Spell the trailer key as `Co-authored-by`.
- Order the trailers by name in alphabetical order.
- Add 1 trailer for each person or agent, other than the commit author, who contributed to the commit.
- For a person, use their name and the email that GitHub links to their account, such as their `users.noreply.github.com` address. For an agent, use the name and email in the attribution trailer that its harness gives. If the harness gives none, use the identity in [Agent trailers](#agent-trailers).
- Keep the existing trailers when you amend or squash commits.
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

Follow these rules for each commit that reverts an earlier commit, including the squash commit of a PR that reverts a commit.

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

This checkpoint names the tests of its step and uses `Refs` to keep its Issue open until the squash commit:

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

### Feature squash commits

A feature squash commit has no body when it builds what its Issue and specification describe and adds no migration, configuration, or decision of its own:

```text
feat(identity): validate Google callbacks and issue handoff codes

Closes: #217

Co-authored-by: Claude Opus 5.5 <noreply@anthropic.com>
```

A feature squash commit has a body when it adds a migration, configuration, or decision that the specification does not state:

```text
feat(identity): create provider-only accounts from Google logins

Migration 00008 allows an empty password hash. Password login and
recovery skip such accounts, and rolling back the migration fails
while they exist.

Closes: #219

Co-authored-by: Claude Opus 5.5 <noreply@anthropic.com>
```

### Breaking squash commits

This breaking squash commit has a breaking marker in its subject and explains the incompatibility and the changes that callers must make in its body:

```text
feat(workspace)!: require a role when inviting members

InviteMember rejects a request without a role with InvalidArgument.
Callers must send the role field, which was optional before.

Co-authored-by: Claude Opus 5.5 <noreply@anthropic.com>
```

### Test squash commits

A squash commit that only adds tests has no body, because the subject states every lasting effect:

```text
test(observability): prove logs and traces against the specification

Closes: #306

Co-authored-by: Claude Opus 5.5 <noreply@anthropic.com>
```

### Document squash commits

A squash commit that only adds a specification has no body, because the specification on `main` holds the content:

```text
docs(observability): approve observability metrics and dashboards spec

Co-authored-by: Claude Opus 5.5 <noreply@anthropic.com>
```

This ADR squash commit has no body, because the new record states the reason in its [Context](adrs.md#context) and [Decision](adrs.md#decision) sections and the earlier record only changes its status:

```text
docs(adr): record local alert routing

Co-authored-by: Claude Opus 5.5 <noreply@anthropic.com>
```

A squash commit that approves a plan has no body and no `Refs` footers, because the plan on `main` links each of its Issues:

```text
docs(observability): approve observability logs and traces plan

Co-authored-by: Claude Opus 5.5 <noreply@anthropic.com>
```

This squash commit states only the reason in its body, because the changed convention states the new rules:

```text
docs(agents): revise module specification sections and approval

Specifications repeated commands that Issue verification steps and
CONSTRAINTS.md already own. Some section names did not say what
belongs in them.

Co-authored-by: Claude Opus 5.5 <noreply@anthropic.com>
```

### Revert commits

This revert commit keeps the scope and description of the reverted commit and names the reverted commit and the reason in its body:

```text
revert(agents): clarify convention headings

This reverts commit 2f77dde4c1b9a6e3d5f8a0b7c2e4d6f8a1b3c5e7.

The new headings broke links from other convention files.

Co-authored-by: Claude Opus 5.5 <noreply@anthropic.com>
```
