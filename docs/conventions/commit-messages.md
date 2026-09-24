# Commit message conventions

This convention defines the format for checkpoint and squash commit messages. Follow it when writing a checkpoint commit or preparing a suggested squash commit.

## Template

Use [Conventional Commits 1.0.0](https://www.conventionalcommits.org/en/v1.0.0/). A message has a subject and can include a body and footer. Use this order. Omit the scope, body, and `Refs` footer when they do not apply. Include the co-author trailer for each contributing agent.

```text
<type>(<scope>): <description>

<body>

Refs: #<issue-number>

Co-authored-by: Codex <noreply@openai.com>
```

| Part | How to write it |
| --- | --- |
| Subject | Use `<type>: <description>` or `<type>(<scope>): <description>` on the first line. Write a short, specific description. Do not use vague text such as `update`, `misc`, or `fix things`. If a contract change breaks callers, put `!` before the colon and explain the incompatibility and required caller changes in the body. |
| Body | If the reason or trade-off is unclear, explain it. Include only information that helps the reviewer act. Omit repeated text, process history, abandoned methods, hypothetical objections, and unrelated files. |
| `Refs` footer | If a commit belongs to an Issue, add `Refs: #<issue-number>` before co-author trailers. This links the commit to the Issue without closing it. |

## Rules

Format the message as follows:

- Limit prose lines in the body to 72 characters.
- Preserve paragraphs and lists.
- Do not split URLs, code, or trailers.
- For multiline messages, write the message in a file and run `git commit --file <message-file>` separately.

Record AI co-authorship as follows:

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
