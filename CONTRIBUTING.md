# Contributing to FlowSpace

This policy applies to maintainers and AI agents. It replaces general workflow rules when they conflict with this file.

A pull request (PR) proposes changes for review. A squash merge combines all commits in a PR into one commit.

Agents prepare and test PRs. The maintainer reviews and squash merges PRs. Agents must not merge PRs, enable auto-merge, or push directly to `main`.

## Workflow

The working tree contains local repository files and changes. A branch holds changes outside `main`. A checkpoint commit records one tested work step. A diff shows the changes between two versions.

1. Inspect the working tree and the project instructions.
2. Preserve work that is outside the task.
3. Create a short-lived branch from the current `main`.
4. Keep `main` ready for deployment.
5. Make small changes and test each change.
6. Create checkpoint commits for the tested changes.
7. Keep unrelated refactoring and formatting separate from behavior changes.
8. Complete the [verification requirements](#verification).
9. Open a PR to `main`.
10. Address review comments and rerun the affected commands.
11. After the maintainer merges the PR, remove the branch if it contains no work to preserve.

Keep each branch and PR limited to one reviewable change. Split unrelated changes into separate PRs.

A worktree is a separate checkout of the repository. If tasks run at the same time, use separate worktrees.

## Markdown files

Hard wrapping inserts manual line breaks within paragraphs or list items. Do not hard-wrap Markdown files. Keep each paragraph and list item on one physical line, regardless of length.

## Branch names

Use `<type>/<short-description>` for a branch name. Select a [commit type](#types). Write the description in lowercase and separate its words with hyphens. Do not add an agent or author prefix.

```text
feat/workspace-invitations
fix/duplicate-notifications
docs/git-workflow
ci/pr-title-validation
```

## Commit messages

A commit message has a subject and can include a body and footer. The subject is the first line. The body explains the change. The footer holds metadata, such as co-author trailers.

Use [Conventional Commits 1.0.0](https://www.conventionalcommits.org/en/v1.0.0/) for checkpoint and squash commits. Use this subject format:

```text
<type>: <description>
<type>(<scope>): <description>
```

- Write a short and specific description.
- Do not use vague text such as `update`, `misc`, or `fix things`.
- If the reason or trade-off is unclear, add a body.
- Explain the reason and include only information that helps the reviewer act.
- Omit repeated text, process history, abandoned methods, hypothetical objections, and unrelated files.

### Formatting

- In checkpoint commit bodies and suggested squash messages, limit prose lines to 72 characters.
- Preserve paragraphs and lists. Do not split URLs, code, or trailers.
- For multiline commit messages, apply these rules in a message file. Run `git commit --file <message-file>` separately.

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

A new file alone does not make the change a `feat`.

If a contract change breaks callers, put `!` before the colon. In the body, explain the incompatibility and the required caller changes:

```text
feat(workspace)!: require a role when inviting members
```

### Scopes

A scope names the repository area that a change affects. Use one lowercase scope when it makes the affected area clear:

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

If generated code or OpenAPI output follows a source definition, use the type and scope of that source.

Choose a scope with these steps. Stop after the first matching step:

1. If the main change updates a Protobuf RPC definition or public HTTP annotation, use `proto`.
2. If the main change updates a published Protobuf event schema, use `events`.
3. If the main change updates code-generation configuration or tooling, use `codegen`.
4. If the change belongs to one service, use the service scope.
   - This scope includes related contracts, queries, generated code, tests, configuration, and logging.
   - Code under `services/workspace/internal/bootstrap/` uses `workspace`.
   - For package locations, see the [project structure](docs/project-structure.md).
5. If the change affects one shared technical package under root `internal/`, use its directory name.
   - If you introduce a shared package, add its directory name to the table.
6. If one change affects several shared packages, use `shared`.
7. If the change affects agent instructions, skills, commands, or configuration, use `agents`.
8. If another area in the table fits, use that scope.
9. If no single area fits, omit the scope.

Use these rules for all scopes:

- Reuse an existing scope when it fits.
- If a PR needs a new scope, define the scope in that PR.
- Do not combine scope names.

```text
feat(workspace): allow owners to invite workspace members
fix(work): reject task updates based on an outdated version
docs(adr): explain the choice of squash merging
fix(agents): preserve multiline PR descriptions
build(codegen): configure Buf to generate ConnectRPC clients
ci: add pull request title validation
```

### AI co-authorship

A co-author trailer identifies a contributor at the end of a commit message. Each AI-assisted checkpoint and squash commit must include one trailer for each contributing agent. Use this trailer for Codex:

```text
Co-authored-by: Codex <noreply@openai.com>
```

Put trailers after a blank line at the end of the message. Preserve existing attribution when you amend or squash commits. Use the identity of each agent.

## Pull requests

- Use the [PR template](.github/pull_request_template.md).
- Complete Change, Reason, and Verification.
- Keep each paragraph and list item in a PR description on one physical line.
- If there are no risks or limitations, omit that section.
- Apply the matching `type:*` label.
- Apply relevant `area:*`, `breaking`, and `migration` labels from [`.github/labels.json`](.github/labels.json).
- Keep the title, description, and labels consistent with the final change.

### PR titles

Use only the [commit subject format](#commit-messages), with the same [types](#types) and [scopes](#scopes). Do not include a body or footer in the title. Put details in the PR description.

Write the PR title to describe the result, not the branch or changed files:

```text
Branch: fix/duplicate-notifications
Title:  fix(notifications): prevent duplicate delivery when an event is retried
```

### Verification

Before review, complete these actions:

- Inspect the staged diff before each commit.
- Inspect the complete PR diff before maintainer review.
- Exclude unrelated changes, secrets, local environment files, and unwanted build output.
- Run the relevant tests, lint commands, builds, and contract commands.
- For a behavior fix, add a focused regression test.
- For a contract or generator change, make sure that regeneration and compatibility succeed.
- For these changes, make sure that source files and generated output are consistent.
- For a documentation-only change, make sure that facts, examples, links, and formatting are correct.
- For documentation-only changes, application tests are unnecessary unless executable behavior changes.
- Do not weaken commands, hide failures, or discard work from another task.

For each check, use the matching `task ...` command from `Taskfile.yaml` first. If no matching task exists, use another command. Record each required command and its exact result in the PR Verification table, including commands outside Task. If a command did not run, record the reason. Do not report that command as passing. If a command fails, fix the failure or mark the PR as needing attention.

### Suggested squash commit

Prepare the suggested squash message with these rules:

- Before maintainer review, provide the exact suggested squash message in the chat.
- Do not include the suggested squash message in the PR description.
- If the PR changes, update the message.
- Use the reviewed PR title as the subject.
- Follow the [message rules](#commit-messages), [formatting rules](#formatting), and [AI attribution rules](#ai-co-authorship).
- If a body is needed, explain the important effects and compatibility or migration information.
- Do not copy detailed verification from the PR into the body.
- Use the GitHub Pull request title and description squash default.

The maintainer can shorten the copied description but must keep required context and trailers.
