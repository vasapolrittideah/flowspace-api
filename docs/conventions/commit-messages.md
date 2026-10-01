# Commit message conventions

This convention defines the format for checkpoint and squash commit messages.

## Template

A message has a subject, an optional body, and optional footers. The scope and Issue footers are optional.

```text
<type>(<scope>): <description>

<body>

Closes: #<issue-number>
Refs: #<issue-number>

Co-authored-by: <agent-name> <agent-email>
```

- Subject: A type, optional scope, and short description on the first line. The `!` marker denotes a breaking contract change.
- Body: The reason or trade-off when it is not clear from the subject. For a breaking contract change, the incompatibility and required caller changes.
- `Closes` footer: An Issue that the pull request completes. Use it only in a squash commit. GitHub closes the Issue when the commit reaches `main`.
- `Refs` footer: A related Issue that stays open. This footer links the commit to the Issue without closing it.
- Co-author trailer: The identity of each contributing agent.

## Rules

### Message format

- Follow [Conventional Commits 1.0.0](https://www.conventionalcommits.org/en/v1.0.0/) and write the subject, body, and footers in the order shown in the template.
- If a contract change breaks callers, put `!` before the colon and explain the incompatibility and required caller changes in the body.
- Write a short, specific subject. Do not use vague text such as `update`, `misc`, or `fix things`.
- If a checkpoint commit belongs to an Issue, add `Refs: #<issue-number>` before co-author trailers.
- In a squash commit, copy the pull request's `Related issues` as footers: `Closes: #<issue-number>` for each Issue it completes and `Refs: #<issue-number>` for each Issue that stays open. Do not add Issues from `Follow-up tasks`.
- Write one footer per Issue, and repeat the key on each line. Put `Closes` footers before `Refs` footers, and order each group by ascending Issue number. Do not leave blank lines between Issue footers or add a period at the end.
- Omit the scope, body, and Issue footers when they do not apply.
- For multiline messages, write the message in a file and run `git commit --file <message-file>` separately.
- Limit prose lines in the body to 72 characters.
- Preserve paragraphs and lists.
- Do not split URLs, code, or trailers.

### Body content

- Write a body only when it gives information that the subject does not. Omit repeated text, process history, abandoned methods, hypothetical objections, and unrelated files.
- In a checkpoint commit, write for a reviewer who reads the PR one commit at a time. Explain what the step changes, and the reason or trade-off when the subject does not make it clear. You can name the tests that the step adds, because they are part of the step.
- In a squash commit, write for a later reader of the `main` history. Describe only the effects that stay on `main` after the merge: changed behavior, compatibility or migration effects, and important decisions or trade-offs. Do not include verification details, lists of added tests, specification or plan status changes, or process history. If the subject states every lasting effect, such as in a PR that only adds tests, omit the body.
- Build a squash body from the checkpoint bodies. Keep each sentence that describes a lasting effect on `main`, without rewording it, and remove the other sentences. Write a new sentence only for a lasting effect of the final change that no checkpoint body states.

### AI co-authorship

- Include one co-author trailer for each agent that contributes to an AI-assisted checkpoint or squash commit.
- Preserve existing attribution when you amend or squash commits.
- Use the identity of the agent that contributes. If the agent's harness gives an attribution trailer, use the name and email in that trailer. Otherwise, use the identity in [Agent trailers](#agent-trailers). Always spell the trailer key as `Co-authored-by`.
- Do not add a trailer for an agent that did not contribute.
- Put trailers after a blank line at the end of the message.

### Type and scope choice

A new file alone does not make the change a `feat`.

If generated code or OpenAPI output follows a source definition, use the type and scope of that source. Choose a scope with these steps and stop after the first match:

1. If the main change updates a Protobuf RPC definition or public HTTP annotation, use `proto`.
2. If the main change updates a published Protobuf event schema, use `events`.
3. If the main change updates code-generation configuration or tooling, use `codegen`.
4. If the change belongs to one service, use the service scope. This includes related contracts, queries, generated code, tests, configuration, and logging. Code under `services/<service>/internal/bootstrap/` uses the scope of that service. For package locations, see the [project structure](../project-structure.md).
5. If the change affects one shared technical package under root `internal/`, use its directory name. If you introduce a shared package, add its directory name to the table.
6. If one change affects several shared packages, use `shared`.
7. If the change affects agent instructions, skills, commands, or configuration, use `agents`.
8. If another area in the table fits, use that scope.
9. If no single area fits, omit the scope.

Reuse an existing scope when it fits. If a PR needs a new scope, define the scope in that PR. Do not combine scope names.

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

### Scopes

A scope names the repository area that a change affects. Use one lowercase scope when it makes the affected area clear:

| Scope | Area |
| --- | --- |
| `authn` | Shared access token and live session verification under `internal/authn/` |
| `config` | Shared environment configuration under `internal/config/` |
| `logging` | Shared structured logging under `internal/logging/` |
| `postgrespool` | Shared PostgreSQL startup connections under `internal/postgrespool/` |
| `requestid` | Shared request ID validation and generation under `internal/requestid/` |
| `identity` | Accounts, authentication, email verification, and sessions |
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

### Agent trailers

| Agent | Trailer |
| --- | --- |
| Codex | `Co-authored-by: Codex <noreply@openai.com>` |
| Claude Code | `Co-authored-by: Claude <noreply@anthropic.com>` |

## Examples

These subjects show the type and scope choices:

```text
feat(workspace): allow owners to invite workspace members
fix(work): reject task updates based on an outdated version
docs(adr): explain the choice of squash merging
fix(agents): preserve multiline PR descriptions
build(codegen): configure Buf to generate ConnectRPC clients
ci: add pull request title validation
feat(workspace)!: require a role when inviting members
```

A documentation checkpoint with Codex attribution uses the full message. Other agents use their own trailer:

```text
docs(agents): clarify convention headings

Co-authored-by: Codex <noreply@openai.com>
```
