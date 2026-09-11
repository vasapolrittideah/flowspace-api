# Contributing to FlowSpace

This policy applies to the maintainer and to AI agents:

- Agents prepare and test pull requests.
- The maintainer reviews and squash merges pull requests.
- Agents must not merge, enable auto-merge, or push directly to `main`.

These rules override the defaults of a generic workflow skill. This includes branch prefixes and preferences against squash merging.

## Development workflow

1. Inspect the working tree and the project instructions. Preserve all work outside the task.
2. Create a short-lived branch from the current `main` for one focused change. Keep `main` deployable.
3. Work in small, verified increments with checkpoint commits. Keep unrelated refactoring and formatting apart from behavior changes.
4. Review the complete diff and run the relevant checks before you declare the change ready.
5. Open a PR to `main` with a descriptive title and evidence of verification. Answer the feedback and run the affected checks again.
6. After the maintainer merges, delete the branch. If it holds work to preserve, keep it.

Keep each PR to one reviewable change. Split unrelated concerns instead of enforcing a limit on the number of lines. If concurrent tasks need isolation, use separate worktrees.

## Branch names

Use `<type>/<short-description>`. Choose a [commit type](#types) and write a lowercase description with hyphens between the words. Do not add a prefix for the agent or the author.

```text
feat/workspace-invitations
fix/duplicate-notifications
docs/git-workflow
ci/pr-title-validation
```

The branch name identifies the work. The PR title describes the result and does not have to repeat the branch name.

## Commit messages and PR titles

Use [Conventional Commits 1.0.0](https://www.conventionalcommits.org/en/v1.0.0/) for checkpoint commits and for PR titles:

```text
<type>: <description>
<type>(<scope>): <description>
```

- Write a short, specific description. Do not write vague text such as `update`, `misc`, or `fix things`.
- If the title does not make the reason or the trade-off clear, add a body.
- Explain why you made the change.
- Include only information that a reviewer can act on. Omit repetition, the history of the process, and arguments against objections that nobody made.
- Apply the same standard to PR descriptions.

### Formatting

- Wrap the body prose of checkpoint commits and suggested squash messages at 72 columns. Keep paragraphs, lists, URLs, code, and trailers intact.
- Never hard-wrap a PR description. Keep each paragraph and each list item on one physical line, whatever its length.

For a message with more than one line:

1. Prepare the message in a file.
2. Make sure that no body line is longer than 72 columns.
3. Commit with `git commit --file <message-file>`.
4. Write the file and commit it in separate steps.
5. Never combine a heredoc with `git commit` or `gh pr create`. The hook in `.claude/settings.json` rejects the command. It also rejects an unrelated heredoc that mentions either command.

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
| `chore` | Maintain the project. No other type fits |

Mark a breaking change to a contract with `!` before the colon. In the body, explain the incompatibility and the changes that callers must make:

```text
feat(workspace)!: require a role when inviting members
```

Release versioning is outside this policy.

### Scopes

If one lowercase scope makes the affected area clear, use it.

| Scope | Area |
| --- | --- |
| `workspace` | Workspaces, memberships, invitations, roles, and authorization |
| `work` | Projects, tasks, assignments, status transitions, comments, activity history, and the event outbox |
| `notifications` | In-app notification inbox, read state, and event deduplication |
| `shared` | Changes across several shared Go packages under root `internal/` |
| `proto` | Protobuf RPC definitions and public HTTP annotations |
| `events` | Published Protobuf event schemas |
| `codegen` | Code-generation configuration and tooling |
| `infra` | Infrastructure and deployment configuration |
| `deps` | Dependency updates |
| `adr` | Architecture decision records |
| `agents` | Agent instructions, skills, commands, and configuration |

Choose a scope in this order:

1. Use the scope of the service for a change to that service. This includes its contracts, its queries, its generated code, and its tests.
2. If the contract itself is the focus, use `proto` or `events`.
3. If the generation tooling or its configuration is the focus, use `codegen`. In every other case, use the matching area of the repository above.
4. If no single area helps, omit the scope. This includes a coherent change across the whole repository. Split unrelated work instead of joining scope names.

For one shared technical package under root `internal/`, use the name of its directory. For example, use `config` for `internal/config/` and `logging` for `internal/logging/`. When you introduce the package, add that scope to the table. Use `shared` only for one coherent change across several shared packages. A change to the configuration or the logging of one service keeps the scope of that service. This includes code under `services/workspace/internal/bootstrap/`. For more information, see the [repository layout](docs/adr/002-repository-and-go-module-layout.md).

Reuse the scope names that exist. Propose and document a new scope in the PR that needs it. `ci` is a type, not a scope. Write `ci: ...` or name an area, such as `ci(work): ...`. Documentation uses `docs` with an optional scope for the affected area. A change to an agent uses the `agents` scope and the type that matches its purpose. A new file alone is not always a feature.

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

Generated code and OpenAPI output belong with the definition they come from. They use the type and the scope of that change. There is no `generated` scope and no `openapi` scope. Use `codegen` only for the configuration or the tooling of generation. The internal RPC clients of FlowSpace use ConnectRPC.

### AI co-authorship

Every AI-assisted checkpoint commit and squash commit must include one `Co-authored-by` trailer for each agent that contributed. For Codex:

```text
Co-authored-by: Codex <noreply@openai.com>
```

Put the trailers at the end of the message, after a blank line. When you amend or squash a commit, preserve the attribution that exists. Use the identity of each agent.

## Pull requests

### Title and description

Use the intended final squash commit subject as the PR title. Describe the result, not the branch and not the changed files:

```text
Branch: fix/duplicate-notifications
Title:  fix(notifications): prevent duplicate delivery when an event is retried
```

- Use the [PR template](.github/pull_request_template.md).
- Fill in Change, Reason, and Verification. If no risk or limitation applies, omit Risks or limitations.
- Treat the HTML comments in the template as guidance for the writer. They do not render.
- Apply the matching `type:*` label. Also apply the `area:*`, `breaking`, and `migration` labels from [`.github/labels.json`](.github/labels.json) that apply.
- Keep the labels, the title, and the description current as the PR changes.
- Omit the history of the conversation, the approaches you abandoned, and lists of unrelated files that you did not touch.

### Verification

Before review:

- Inspect the staged diff before each commit, and the complete PR diff before review. Exclude unrelated changes, secrets, local environment files, and unwanted build output.
- Run the relevant tests, linting, builds, and contract checks. Add a focused regression test for a fix to behavior.
- If you change a contract or a generator, make sure that regeneration succeeds and that compatibility holds. Keep the sources and the generated output consistent.
- For a documentation-only change, review the accuracy, the examples, the links, and the formatting. You do not have to run application tests unless executable behavior changes.
- Never weaken a check, hide a failure, or discard the work of another task to look ready.

Record every warranted check in the Verification table of the PR with its exact command and its result. Include the checks that did not run and explain why. Never report a check that did not run as passing. Resolve the failures, or mark the PR as one that needs attention.

### Squash commit message

Summarize the final result of the PR one time, whatever its checkpoint commits contain:

1. Subject: the reviewed Conventional Commit PR title.
2. Body: why the change was necessary, its important effects, and the notes about compatibility or migration. Wrap the prose at 72 columns. Omit checkpoint messages, abandoned approaches, repeated descriptions, and the verification detail that the PR already records.
3. Trailers: each distinct co-author one time, after a blank line. This includes the required AI attribution.

Before handoff, do the following:

1. Review the complete diff.
2. Align the title and the description with it.
3. Make sure that the body wrapping and the trailers are correct.
4. Give the exact suggested squash message.

Refresh that message after a relevant change.

Use the GitHub squash default named "Pull request title and description". The copied description is a starting point. The maintainer shortens it and keeps the necessary context and trailers. The copy is not an approval of the message, and it does not authorize a merge.

This document defines the workflow. It does not define GitHub protections, automation, deployment gates, or releases.
