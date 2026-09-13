# Contributing to FlowSpace

This policy applies to the maintainer and AI agents:

- Agents prepare and test pull requests.
- The maintainer reviews and squash merges pull requests.
- Agents must not merge, enable auto-merge, or push directly to `main`.

These rules override generic workflow skill defaults, including branch prefixes and preferences against squash merging.

## Development workflow

1. Inspect the working tree and project instructions. Preserve work outside the task.
2. Create a short-lived branch from the current `main` for one focused change. Keep `main` deployable.
3. Work in small, verified increments with checkpoint commits. Keep unrelated refactoring and formatting separate from behavior changes.
4. Review the complete diff and run relevant checks before declaring the change ready.
5. Open a PR to `main` with a descriptive title and verification evidence. Address feedback and rerun affected checks.
6. After the maintainer merges, remove the branch when it has no work left to preserve.

Keep each PR to one reviewable change; split unrelated concerns rather than enforcing a line limit. Use separate worktrees when concurrent tasks need isolation.

## Branch names

Use `<type>/<short-description>`, choosing a [commit type](#types) and a lowercase, hyphen-separated description. Do not add an agent or author prefix.

```text
feat/workspace-invitations
fix/duplicate-notifications
docs/git-workflow
ci/pr-title-validation
```

The branch name identifies the work; the PR title describes the result and need not repeat it.

## Commit messages and PR titles

Use [Conventional Commits 1.0.0](https://www.conventionalcommits.org/en/v1.0.0/) for checkpoint commits and PR titles:

```text
<type>: <description>
<type>(<scope>): <description>
```

- Use a concise description, not vague text such as `update`, `misc`, or `fix things`.
- Add a body only when the reason or trade-off is unclear from the title.
- Explain why, include only information a reviewer can act on, and omit repetition, process history, and arguments against hypothetical objections.
- Apply the same standard to PR descriptions.

### Formatting

- Wrap body prose in checkpoint commits and suggested squash messages at 72 columns while preserving paragraphs, lists, URLs, code, and trailers.
- Never hard-wrap PR descriptions. Keep each paragraph and list item on one physical line, regardless of length.

For multiline messages:

- Prepare the message in a file.
- Check its line lengths.
- Use `git commit --file <message-file>`.
- Write and commit in separate steps.
- Never combine a heredoc with `git commit` or `gh pr create`; the `.claude/settings.json` hook rejects the command, including unrelated heredocs that mention either command.

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

Mark a breaking contract change with `!` before the colon and explain the incompatibility and required caller changes in the body:

```text
feat(workspace)!: require a role when inviting members
```

Release versioning is outside this policy.

### Scopes

Use one lowercase scope when it clarifies the affected area.

| Scope | Area |
| --- | --- |
| `config` | Shared environment configuration under `internal/config/` |
| `logging` | Shared structured logging under `internal/logging/` |
| `workspace` | Workspaces, memberships, invitations, roles, and authorization |
| `work` | Projects, tasks, assignments, status transitions, comments, activity history, and the event outbox |
| `notifications` | In-app notification inbox, read state, and event deduplication |
| `shared` | Changes spanning several shared Go packages under root `internal/` |
| `proto` | Protobuf RPC definitions and public HTTP annotations |
| `events` | Published Protobuf event schemas |
| `codegen` | Code-generation configuration and tooling |
| `infra` | Infrastructure and deployment configuration |
| `deps` | Dependency updates |
| `adr` | Architecture decision records |
| `agents` | Agent instructions, skills, commands, and configuration |

Choose a scope in this order:

1. Use the service scope for a service change, including its contracts, queries, generated code, and tests.
2. Use `proto` or `events` when the corresponding contract itself is the focus.
3. Use `codegen` when generation tooling or configuration is the focus; otherwise use the matching repository area above.
4. Omit the scope when no single area helps, including coherent repository-wide changes. Split unrelated work instead of joining scope names.

For one shared technical package under root `internal/`, use its directory name, such as `config` for `internal/config/` or `logging` for `internal/logging/`, and add that scope to the table when introducing the package. Use `shared` only for a coherent change across several shared packages. Changes to one service's settings or logging retain the service scope, including code under `services/workspace/internal/bootstrap/`; see the [codebase structure](docs/codebase-structure.md).

Reuse existing scope names. Propose and document a new scope in the PR that needs it. `ci` is a type, not a scope; use `ci: ...` or an area such as `ci(work): ...`. Documentation uses `docs` with an optional affected-area scope. Agent changes use the `agents` scope and the type matching their purpose; adding a file alone is not necessarily a feature.

Examples:

```text
feat(workspace): allow owners to invite workspace members
fix(work): reject task updates based on an outdated version
feat(config): support loading settings from environment variables
refactor(shared): standardize initialization across shared packages
docs(adr): explain the choice of squash merging
fix(agents): preserve multiline PR descriptions
feat(events): add correlation IDs to domain events
fix(proto): document the conflict response for stale task updates
build(codegen): configure Buf to generate ConnectRPC clients
chore(deps): update Go dependencies
ci: add pull request title validation
```

Generated code and OpenAPI output belong with their source definition and use that change's type and scope; there is no `generated` or `openapi` scope. Use `codegen` only for generation configuration or tooling. FlowSpace's internal RPC clients use ConnectRPC.

### AI co-authorship

Every AI-assisted checkpoint and squash commit must include a `Co-authored-by` trailer for each contributing agent. For Codex:

```text
Co-authored-by: Codex <noreply@openai.com>
```

Place trailers after a blank line at the end of the message. Preserve existing attribution when amending or squashing, and use each agent's own identity.

## Pull requests

### Title and description

Use the intended final squash commit subject as the PR title and describe the result rather than the branch or changed files:

```text
Branch: fix/duplicate-notifications
Title:  fix(notifications): prevent duplicate delivery when an event is retried
```

- Use the [PR template](.github/pull_request_template.md).
- Fill in Change, Reason, and Verification; omit Risks or limitations when none apply.
- Treat the template's HTML comments as editing guidance; they do not render.
- Apply the matching `type:*` label and any applicable `area:*`, `breaking`, and `migration` labels from [`.github/labels.json`](.github/labels.json); keep them current as the PR changes.
- Keep the title and description current with the final implementation.
- Omit conversation history, abandoned approaches, and lists of unrelated untouched files.

### Verification

Before review:

- Inspect the staged diff before each commit and the complete PR diff before review. Exclude unrelated changes, secrets, local environment files, and unwanted build output.
- Run relevant tests, linting, builds, and contract checks. Add a focused regression check for behavior fixes.
- For contract or generator changes, verify regeneration and compatibility, and keep sources and generated output consistent.
- For documentation-only changes, review accuracy, examples, links, and formatting. Application tests are unnecessary unless executable behavior changes.
- Never weaken checks, suppress failures, or discard another task's work to appear ready.

Record every warranted check in the PR's Verification table with its exact command and result. Include checks that did not run and explain why; never report an unrun check as passing. Resolve failures or mark the PR as needing attention.

### Squash commit message

Summarize the PR's final result once, regardless of its checkpoint commits:

- **Subject:** The reviewed Conventional Commit PR title.
- **Body:** Why the change was needed, its important effects, and compatibility or migration notes. Wrap prose at 72 columns. Omit checkpoint messages, abandoned approaches, repeated descriptions, and detailed verification already in the PR.
- **Trailers:** Each distinct co-author once, including required AI attribution, after a blank line.

Before handoff, review the complete diff, align the title and description, verify body wrapping and trailers, and provide the exact suggested squash message. Refresh it after relevant changes.

Use GitHub's **Pull request title and description** squash default. The copied description is a starting point for the maintainer to shorten while retaining necessary context and trailers; it neither validates the message nor authorizes a merge.

This document defines the workflow, not GitHub protections, automation, deployment gates, or releases.
