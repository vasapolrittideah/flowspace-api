# Commit message conventions

This convention defines the format for checkpoint and squash commit messages.

## Template

A message has a subject line, a body, Issue footers, and co-author trailers.

```text
<type of change>(<scope, if any>): <description of the change>

<reason or trade-off that the subject does not state, if any>

Closes: #<Issue that the change completes, if any>
Refs: #<related Issue that stays open, if any>

Co-authored-by: <agent name> <agent email>
```

### Type

- Use one of the [types](#types).
- If generated code or OpenAPI output follows a source definition, use the type of that source.
- If a contract change breaks callers, put `!` before the colon.
- Do not use `feat` only because the change adds a file.

### Scope

- Choose the scope with these steps, and stop after the first match:
  1. If the main change updates a Protobuf RPC definition or public HTTP annotation, use `proto`.
  2. If the main change updates a published Protobuf event schema, use `events`.
  3. If the main change updates code-generation configuration or tooling, use `codegen`.
  4. If the change belongs to one service, use the service scope. This includes related contracts, queries, generated code, tests, configuration, and logging. Code under `services/<service>/internal/bootstrap/` uses the scope of that service. For package locations, see the [project structure](../project-structure.md).
  5. If the change affects one shared technical package under root `internal/`, use its directory name. If you introduce a shared package, add its directory name to the table.
  6. If one change affects several shared packages, use `shared`.
  7. If the change affects agent instructions, skills, commands, or configuration, use `agents`.
  8. If another area in the table fits, use that scope.
  9. If no single area fits, omit the scope.
- If generated code or OpenAPI output follows a source definition, use the scope of that source.
- Reuse an existing [scope](#scopes) when it fits. If a PR needs a new scope, define the scope in that PR.
- Do not combine scope names.

### Description

- Write a short, specific description.
- Do not use vague text such as `update`, `misc`, or `fix things`.

### Body

- Limit prose lines to 72 characters. Preserve paragraphs and lists.
- Write a body only when it gives information that the subject does not.
- If the subject has `!`, explain the incompatibility and the required caller changes.
- In a checkpoint commit, write for a reviewer who reads the PR one commit at a time. Explain what the step changes, and the reason or trade-off when the subject does not make it clear. You can name the tests that the step adds, because they are part of the step.
- In a squash commit, write for a later reader of the `main` history. Describe only the effects that stay on `main` after the merge: changed behavior, compatibility or migration effects, and important decisions or trade-offs. If the subject states every lasting effect, such as in a PR that only adds tests, omit the body.
- In a squash commit, if the change only adds a document or changes its status, such as a specification, plan, or ADR, omit the body. The document on `main` holds its content. Write a body when the change alters an existing rule or behavior and the changed files do not state the reason.
- If a squash commit needs a body, summarize the final `What changed` and `Why` sections of the pull request. Make sure that the final diff supports each sentence.
- Do not split URLs, code, or trailers.
- Do not repeat text or add process history, abandoned methods, hypothetical objections, or unrelated files.
- In a squash commit, do not include verification details, lists of added tests, specification or plan status changes, or process history.
- In a squash commit, do not copy text that only reviewers need, such as merge order, review notes, checks that did not run, or follow-up tasks.

### Issue footers

- Write one footer per Issue, and repeat the key on each line.
- Put `Closes` footers before `Refs` footers, and order each group by ascending Issue number.
- Use `Closes` only in a squash commit. GitHub closes the Issue when the commit reaches `main`.
- If a checkpoint commit belongs to an Issue, add a `Refs` footer for it.
- In a squash commit, copy the pull request's `Related issues` as footers: `Closes` for each Issue it completes and `Refs` for each Issue that stays open.
- Do not leave blank lines between Issue footers or add a period at the end.
- Do not add Issues from `Follow-up tasks`.

### Co-author trailer

- Put the trailers after a blank line at the end of the message.
- Include one trailer for each agent that contributes to an AI-assisted checkpoint or squash commit.
- Use the identity of the agent that contributes. If the agent's harness gives an attribution trailer, use the name and email in that trailer. Otherwise, use the identity in [Agent trailers](#agent-trailers).
- Always spell the trailer key as `Co-authored-by`.
- Preserve existing attribution when you amend or squash commits.
- Do not add a trailer for an agent that did not contribute.

## Rules

- Follow [Conventional Commits 1.0.0](https://www.conventionalcommits.org/en/v1.0.0/) and write the parts in the order shown in the template.
- For multiline messages, write the message in a file and run `git commit --file <message-file>` separately.

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

A squash commit that only adds a specification has no body, because the specification on `main` holds the content:

```text
docs: specify local observability metrics and dashboards

Co-authored-by: Claude Opus 5.5 <noreply@anthropic.com>
```

A squash commit that changes existing rules has a body that states the new rules:

```text
docs: revise module specification sections and approval

Specifications drop the Commands section because Issue verification
steps and CONSTRAINTS.md own commands. Five sections get clearer
names, and each section states what belongs in it and what does not.

Co-authored-by: Claude Opus 5.5 <noreply@anthropic.com>
```
