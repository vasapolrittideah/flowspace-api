# Commit message conventions

## Message parts

A commit message has a subject and can include a body and footer:

| Part | Content |
| --- | --- |
| `Subject` | First line of the message |
| `Body` (optional) | Explanation of the change |
| `Footer` (optional) | Metadata, such as co-author trailers |

## Subject

Use [Conventional Commits 1.0.0](https://www.conventionalcommits.org/en/v1.0.0/) for checkpoint and squash commits. Use this subject format:

```text
<type>: <description>
<type>(<scope>): <description>
```

- Write a short and specific description. Do not use vague text such as `update`, `misc`, or `fix things`.
- If the reason or trade-off is unclear, add a body. Explain the reason and include only information that helps the reviewer act.
- Omit repeated text, process history, abandoned methods, hypothetical objections, and unrelated files.

## Formatting

- In commit bodies, limit prose lines to 72 characters. Preserve paragraphs and lists. Do not split URLs, code, or trailers.
- For multiline commit messages, apply these rules in a message file. Run `git commit --file <message-file>` separately.
- If a commit belongs to an issue, add `Refs: #<issue-number>` as a footer before the co-author trailers. This reference links the commit to the issue without closing it.

## Types

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

## Scopes

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
4. If the change belongs to one service, use the service scope. This scope includes related contracts, queries, generated code, tests, configuration, and logging. Code under `services/workspace/internal/bootstrap/` uses `workspace`. For package locations, see the [project structure](../project-structure.md).
5. If the change affects one shared technical package under root `internal/`, use its directory name. If you introduce a shared package, add its directory name to the table.
6. If one change affects several shared packages, use `shared`.
7. If the change affects agent instructions, skills, commands, or configuration, use `agents`.
8. If another area in the table fits, use that scope.
9. If no single area fits, omit the scope.

Use these rules for all scopes:

- Reuse an existing scope when it fits. If a PR needs a new scope, define the scope in that PR.
- Do not combine scope names.

```text
feat(workspace): allow owners to invite workspace members
fix(work): reject task updates based on an outdated version
docs(adr): explain the choice of squash merging
fix(agents): preserve multiline PR descriptions
build(codegen): configure Buf to generate ConnectRPC clients
ci: add pull request title validation
```

## AI co-authorship

A co-author trailer identifies a contributor at the end of a commit message. Each AI-assisted checkpoint and squash commit must include one trailer for each contributing agent. Use this trailer for Codex:

```text
Co-authored-by: Codex <noreply@openai.com>
```

Put trailers after a blank line at the end of the message. Preserve existing attribution when you amend or squash commits. Use the identity of each agent.
