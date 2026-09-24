# Commit message conventions

This convention defines the format for checkpoint and squash commit messages.

## Template

A message has a subject, an optional body, and optional footers. The scope and `Refs` footer are optional.

```text
<type>(<scope>): <description>

<body>

Refs: #<issue-number>

Co-authored-by: Codex <noreply@openai.com>
```

| Part | Content and format |
| --- | --- |
| Subject | A type, optional scope, and short description on the first line. The `!` marker denotes a breaking contract change. |
| Body | The reason or trade-off when it is not clear from the subject. For a breaking contract change, the incompatibility and required caller changes. |
| `Refs` footer | The related Issue number. This footer links the commit to the Issue without closing it. |
| Co-author trailer | The identity of each contributing agent. |

## Rules

Follow this convention when writing a checkpoint commit or preparing a suggested squash commit.

### Message format

- Follow [Conventional Commits 1.0.0](https://www.conventionalcommits.org/en/v1.0.0/) and write the subject, body, and footers in the order shown in the template.
- Omit the scope, body, and `Refs` footer when they do not apply.
- Write a short, specific subject. Do not use vague text such as `update`, `misc`, or `fix things`.
- If a contract change breaks callers, put `!` before the colon and explain the incompatibility and required caller changes in the body.
- If the reason or trade-off is unclear, explain it in the body. Include only information that helps the reviewer act. Omit repeated text, process history, abandoned methods, hypothetical objections, and unrelated files.
- If a commit belongs to an Issue, add `Refs: #<issue-number>` before co-author trailers.
- Limit prose lines in the body to 72 characters.
- Preserve paragraphs and lists.
- Do not split URLs, code, or trailers.
- For multiline messages, write the message in a file and run `git commit --file <message-file>` separately.

### AI co-authorship

- Include one co-author trailer for each agent that contributes to an AI-assisted checkpoint or squash commit.
- Use `Co-authored-by: Codex <noreply@openai.com>` for Codex. Use each other agent's identity for its trailer.
- Put trailers after a blank line at the end of the message.
- Preserve existing attribution when you amend or squash commits.

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

If generated code or OpenAPI output follows a source definition, use the type and scope of that source. Choose a scope with these steps and stop after the first match:

1. If the main change updates a Protobuf RPC definition or public HTTP annotation, use `proto`.
2. If the main change updates a published Protobuf event schema, use `events`.
3. If the main change updates code-generation configuration or tooling, use `codegen`.
4. If the change belongs to one service, use the service scope. This includes related contracts, queries, generated code, tests, configuration, and logging. Code under `services/workspace/internal/bootstrap/` uses `workspace`. For package locations, see the [project structure](../project-structure.md).
5. If the change affects one shared technical package under root `internal/`, use its directory name. If you introduce a shared package, add its directory name to the table.
6. If one change affects several shared packages, use `shared`.
7. If the change affects agent instructions, skills, commands, or configuration, use `agents`.
8. If another area in the table fits, use that scope.
9. If no single area fits, omit the scope.

Reuse an existing scope when it fits. If a PR needs a new scope, define the scope in that PR. Do not combine scope names.

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

A documentation checkpoint with Codex attribution uses the full message:

```text
docs(agents): clarify convention headings

Co-authored-by: Codex <noreply@openai.com>
```
