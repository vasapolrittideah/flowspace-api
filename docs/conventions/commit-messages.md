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
- If a change only edits Markdown agent instructions, conventions, skills, or commands, use `docs`. If it changes code or configuration for agents, such as a script or a hook, use the type of that change.
- Do not use `feat` only because the change adds a file.

### Scope

- Choose the scope with these steps, and stop after the first match:
  1. If the main change updates a Protobuf RPC definition or public HTTP annotation, use `proto`.
  2. If the main change updates a published Protobuf event schema, use `events`.
  3. If the main change updates code-generation configuration or tooling, use `codegen`.
  4. If the change only adds or updates ADRs, use `adr`.
  5. If the change affects `AGENTS.md`, `docs/conventions/`, or files under `.agents/`, use `agents`. This includes agent instructions, conventions, skills, commands, and agent configuration.
  6. If the change belongs to one service, use the service scope. This includes related contracts, queries, generated code, tests, configuration, logging, specifications, plans, and runbooks. Code under `services/<service>/internal/bootstrap/` uses the scope of that service. For package locations, see the [project structure](../project-structure.md).
  7. If the change affects the telemetry stack, dashboards, alert rules, or the observability specifications, plans, and runbooks, use `observability`. Instrumentation inside one service uses the service scope, and the shared logging package uses `logging`.
  8. If the change affects one shared technical package under root `internal/`, use its directory name. If you introduce a shared package, add its directory name to the table.
  9. If one change affects several shared packages, use `shared`.
  10. If another area in the table fits, use that scope.
  11. If no single area fits, omit the scope.
- If generated code or OpenAPI output follows a source definition, use the scope of that source.
- Reuse an existing [scope](#scopes) when it fits. If a PR needs a new scope, define the scope in that PR.
- Do not combine scope names.

### Description

- Write a short, specific description. Keep the whole subject line at most 72 characters.
- Do not use vague text such as `update`, `misc`, or `fix things`.

### Body

- Limit prose lines to 72 characters. Preserve paragraphs and lists.
- Write a body only when it gives information that the subject does not.
- If the subject has `!`, explain the incompatibility and the required caller changes.
- In a checkpoint commit, write for a reviewer who reads the PR one commit at a time. Explain what the step changes, and the reason or trade-off when the subject does not make it clear. You can name the tests that the step adds, because they are part of the step.
- In a squash commit, write for a later reader of the `main` history. Describe only the effects that stay on `main` after the merge: changed behavior, compatibility or migration effects, and important decisions or trade-offs. If the subject states every lasting effect, such as in a PR that only adds tests, omit the body.
- If a squash commit needs a body, take its content from the final `What changed` and `Why` sections of the pull request. Keep only what the diff and the changed files do not state, and make sure that the final diff supports each sentence. Do not copy the whole description, because the subject links the pull request.
- In a squash commit, if the change only adds a document or changes its status, such as a specification, plan, or ADR, omit the body. The document on `main` holds its content.
- In a squash commit that changes an existing rule in a document, write only the reason or trade-off that the changed files do not state. If the files state the reason, omit the body. Do not list the new rules, because the files on `main` state them.
- Do not split URLs, code, or trailers.
- Do not repeat text or add process history, abandoned methods, hypothetical objections, or unrelated files.
- In a squash commit, do not include verification details, lists of added tests, or specification or plan status changes.
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
| `observability` | Telemetry stack, dashboards, alert rules, and observability specifications, plans, and runbooks |
| `infra` | Infrastructure and deployment configuration |
| `deps` | Dependency updates |
| `adr` | Architecture decision records |
| `agents` | Agent instructions, conventions, skills, commands, and configuration |

### Agent trailers

| Agent | Trailer |
| --- | --- |
| Codex | `Co-authored-by: Codex <noreply@openai.com>` |
| Claude Code | `Co-authored-by: Claude <noreply@anthropic.com>` |

## Examples

### Subjects

These subjects show the type and scope choices:

```text
feat(workspace): allow owners to invite workspace members
fix(work): reject task updates based on an outdated version
test(identity): prove provider login against its specification
docs(observability): specify local logs and traces
docs(adr): explain the choice of squash merging
fix(agents): preserve multiline PR descriptions
build(codegen): configure Buf to generate ConnectRPC clients
ci: add pull request title validation
feat(workspace)!: require a role when inviting members
```

### Checkpoint commits

A code checkpoint explains the step and names the tests that it adds. It refers to its Issue with `Refs`, because the Issue stays open until the squash commit:

```text
feat(identity): store trace context with outbox events

Migration 00009 adds nullable traceparent and tracestate columns to
identity_outbox_events, and the outbox insert stores the context of
the current span. TestOutboxStoresTraceContext checks the stored
values in a PostgreSQL container.

Refs: #304

Co-authored-by: Claude Opus 5.5 <noreply@anthropic.com>
```

A documentation checkpoint with Codex attribution uses the full message. Other agents use their own trailer:

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

A breaking squash commit has `!` in the subject. Its body explains the incompatibility and the changes that callers must make:

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
docs(observability): specify local metrics and dashboards

Co-authored-by: Claude Opus 5.5 <noreply@anthropic.com>
```

A squash commit that adds an ADR has no body, even when the ADR supersedes an earlier record. The new record states the reason in its `Context` and `Decision` sections, and the earlier record only changes its status:

```text
docs(adr): record local alert routing

Co-authored-by: Claude Opus 5.5 <noreply@anthropic.com>
```

A squash commit that approves a plan has no body. It refers to each Issue of the plan, because the Issues stay open:

```text
docs(observability): approve logs and traces plan

Refs: #299
Refs: #300
Refs: #301
Refs: #302
Refs: #303
Refs: #304
Refs: #305
Refs: #306

Co-authored-by: Claude Opus 5.5 <noreply@anthropic.com>
```

A squash commit that changes existing rules has a body that states only the reason. The changed convention states the new rules:

```text
docs(agents): revise module specification sections and approval

Specifications repeated commands that Issue verification steps and
CONSTRAINTS.md already own. Some section names did not say what
belongs in them.

Co-authored-by: Claude Opus 5.5 <noreply@anthropic.com>
```
