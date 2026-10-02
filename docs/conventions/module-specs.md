# Module specification conventions

This convention defines the format, status, and approval of one module's capability specification.

## Template

A specification has the following fields and sections:

```markdown
# Spec: <capability name>

Module id: `<module-id>`

Status: <current state of the specification>

## Objective

<users, the questions or results they need, and the purpose>

## Scope, dependencies, and ADRs

<included and excluded work, module dependencies, and ADRs>

## Contract

<the shape that a consumer sees: interfaces, routes, fields, events, logs, and errors>

## Behavior

<when each effect happens and why: rules, invariants, security, consistency, and operations>

## Testing strategy

<risks, the test level that proves each risk, and the environment that each level needs>

## Implementation boundaries

<implementer actions to always do, ask about, or never do>

## Success criteria

<outcomes required for completion>

## Assumptions and open questions

<assumptions and unresolved decisions, if any>
```

### Title

- Write the capability name after the `Spec:` prefix.

### Module id

- Copy the module ID from the file name, in backticks.

### Status

- Write `Planned`, `Draft`, `Approved`, or `Implemented`.
- Write the same status as the [specification index](../specs/README.md).

### Objective

- Include one sentence about the first users and their data, such as "The first users are API clients, and all data is disposable under ADR-0022."
- Put assumptions in `Assumptions and open questions`.

### Scope, dependencies, and ADRs

- Start with `Depends on: <module-id>, <module-id>.` or `Depends on: none.`
- Name each module as the `Depends on` column of the index lists it.
- Name the included work.
- Write excluded work as a bullet list after "This capability excludes these items:".
- Write each ADR as a bullet that starts with a link to the ADR, followed by a colon and what the ADR decides for this capability.
- Do not list an ADR only because it applies to every API or every event, such as the ADRs that define Protobuf, REST, versions, errors, and event delivery.

### Contract

- Include each thing that a consumer reads. A consumer is an API client, an event consumer, or a developer who reads logs, traces, metrics, dashboards, or alerts.
- Put a shape in the contract and a condition in `Behavior`. A shape is a name, route, field, or value that a consumer can read. A condition states when an effect happens, or why.
- For an API specification, start the contract with "Use package `flowspace.<service>.v1` and service `<Service>`."
- For RPCs and events, use these tables in this order: `Methods`, `Method requirements`, `HTTP-only endpoints`, `Resource fields`, `Published events`, and `Errors`. Omit a table that has no rows.
- Keep the rows of `Methods` and `Method requirements` in the same RPC order.
- For each other thing that a consumer reads, write one `###` subsection, such as `### Log records` or `### Alert rules`. Use a table when the thing has several fields.
- Do not explain the mechanics of Protobuf, REST, versions, or errors again.
- Do not add a column for rate limits, deadlines, caching, or other conditional rules. State them in `Behavior`.

#### Methods

- Write one row for each RPC.

| Column | How to write |
| --- | --- |
| `RPC` | The RPC name in backticks, such as `` `CreateWorkspace` ``. |
| `Public HTTP route` | The HTTP method and path in backticks, such as `` `POST /v1/workspaces` ``. Write path parameters in braces. Write `None (internal)` for an RPC that has no public route. |
| `Request fields` | The Protobuf field names in backticks, separated by commas. Add `(required)` after each required field. Write `None` when the request has no fields. Do not name the request message or describe credentials. |
| `Response fields` | The Protobuf field names in backticks, separated by commas. Write `None` when the response has no fields. Do not name the response message or the HTTP status. Define the fields in `Resource fields` or `Behavior`. |

#### Method requirements

- Write one row for each RPC, in the order of `Methods`.

| Column | How to write |
| --- | --- |
| `RPC` | The RPC name in backticks, as in `Methods`. |
| `Authentication` | One [authentication value](#method-requirement-values). Add an ADR link only when the value comes from an exception or a capability-specific decision, such as `Service mTLS` under ADR-0036. |
| `Retry` | One [retry value](#method-requirement-values). Add the ADR link for `Rejects Idempotency-Key`. If the value depends on a condition, state the condition in `Behavior`. |

#### HTTP-only endpoints

- Use this table only for endpoints that are not RPCs, such as a provider callback.
- Write one row for each endpoint.

| Column | How to write |
| --- | --- |
| `Endpoint` | A short name, such as "Provider callback". |
| `Public HTTP route` | The HTTP method and path in backticks, as in `Methods`. |
| `Authentication` | One [authentication value](#method-requirement-values). |
| `Request` | The query parameters, headers, or body fields that the endpoint reads, in backticks. |
| `Response` | What the endpoint returns, such as a redirect or a page, and the values that it contains. |

#### Resource fields

- Write one table for each resource that a method returns, and put the resource name in the sentence before the table.
- Write one row for each field.

| Column | How to write |
| --- | --- |
| `Protobuf field` | The Protobuf field name in backticks, such as `` `created_at` ``. |
| `JSON field` | The JSON name in backticks, such as `` `createdAt` ``. |
| `Meaning` | What the value is, its format or limits, and `output only` when the server sets it. |

#### Published events

- Write one row for each event that the capability publishes.

| Column | How to write |
| --- | --- |
| `Event` | The Protobuf message name in backticks, such as `` `EmailDeliveryRequested` ``. |
| `Topic` | The broker topic name in backticks. |
| `Fields` | The Protobuf field names in backticks, separated by commas. If the name of a field does not state its meaning, explain the field in `Behavior`. |

#### Errors

- Write one row for each condition that a client can tell apart.
- Put conditions that return the same status for a security reason in one row.

| Column | How to write |
| --- | --- |
| `Condition` | The cause as the client sees it, such as "Missing or invalid authentication". Do not name internal causes. |
| `gRPC status` | The canonical code in backticks, such as `` `InvalidArgument` ``. Write `Not applicable` when only the REST gateway returns the error. |
| `HTTP status` | The HTTP status number, such as `400`. |

### Behavior

- Put subsections for the topics of the capability first.
- After the topic subsections, add these shared subsections in this order when they apply: `### Security and abuse`, `### Data and compatibility`, and `### Diagnostics`.
- If the capability changes a schema or a stored format, state the migration, its effect on running older code, and its rollback under `### Data and compatibility`.
- Under `### Diagnostics`, list the values that logs, traces, metrics, and errors must never contain. Name the log events and fields in `Contract`, not in `Diagnostics`.
- Do not restate the five-second request cap of [ADR-0010](../adr/0010-cap-ordinary-unary-requests-at-five-seconds.md) unless the capability has an exception.
- Do not repeat the contract.

### Testing strategy

- Write the testing strategy as the [testing table](#testing-table).
- Do not repeat each success criterion.
- Do not list commands. Commands belong in the verification steps of each [Issue](github-issues.md#template).

#### Testing table

- Write one row for each material risk.

| Column | How to write |
| --- | --- |
| `Risk` | The failure that the tests must prevent, in one short phrase, such as "A wrong-purpose code verifies an email". In Identity specifications, add the [threat IDs](../security/identity-threat-model.md) in parentheses, such as "(ID-T03, ID-T19)". |
| `Test level` | One of the [test levels](#test-levels). If two levels prove the risk, write both, separated by a comma. |
| `Environment` | The environment of the test level, as the [test levels](#test-levels) table lists it. If two levels prove the risk, write both environments. Do not write commands. |

### Implementation boundaries

- Put each action under `### Always`, `### Ask first`, or `### Never`. Examples are "Ask first before you change the outbox schema" and "Never read another service's database".
- Start each item with a verb.
- Start each `Never` item with "Do not".
- Do not repeat system rules from `Behavior`.
- Do not repeat a rule from the [constraints](../../CONSTRAINTS.md) or from another convention, such as the rule that a schema change needs a new migration file.
- Do not put plan work in this section, such as a measurement to record in the plan.

### Success criteria

- Write a numbered list of observable Given, When, Then outcomes.
- If a specification has more than about 20 success criteria, consider a split into several modules.
- Do not add a success criterion for the repository checks. The [constraints](../../CONSTRAINTS.md) apply to every change.

### Assumptions and open questions

- Resolve or remove each item before approval, as the [status rules](#status-and-approval) state.

## Rules

### Modules and files

- A module is one capability that can be tested on its own. Write its ID in kebab-case as `<service-or-area>-<capability>`, such as `identity-provider-login` or `observability-logs-and-traces`. Do not rename an ID after it appears in the index.
- Save one specification as `docs/specs/<module-id>.md`, and add each module to the [specification index](../specs/README.md).
- If one piece of work needs several modules, add a row for each module to the index with the `Planned` status before you write the first specification. Name the dependencies of each row. The maintainer reviews this map in its own PR.

### Sections

- Use the sections through `Success criteria` in the template order.
- Add `Assumptions and open questions` only when the specification has an assumption or an open question. Add a new top-level section to this convention before you use it in a specification.
- Put a check that a capability must pass before Flowspace stops using disposable data in the `Before real teams` list of the [architecture](../architecture.md#observability-and-recovery), not in the specification.
- Put implementation locations and commands in the plan and its Issues. Repeat a project-wide rule only when it changes observable behavior or completion criteria. Put project-wide rules in their source documents.

### Status and approval

- `Planned` means that the module is in the index and has no specification file yet. `Draft` means that the specification exists and is not approved. A `Draft` specification can merge into `main`, but it does not permit planning.
- Only the maintainer approves a specification. The approval is the merge of a PR that changes the status to `Approved` in the specification and in the index in the same commit. Use the PR title `docs(<scope>): approve <capability> spec`. An agent changes the status to `Approved` only when the maintainer asks for it in chat or in the PR review.
- Before the status changes to `Approved`, resolve each item in `Assumptions and open questions`, or remove the section.
- An approved specification permits planning.
- If a change to an approved or implemented specification affects its contract, behavior, or success criteria, change its status to `Draft` in the same PR. The specification needs a new approval PR with the title `docs(<scope>): reapprove <capability> spec`. A change that only corrects wording keeps the status.
- Change the status to `Implemented` in the PR that proves the last success criterion.
- Keep each `Implemented` specification in the format that it had when it was implemented.

## Differences from the spec-driven-development skill

The [`spec-driven-development` skill](../../.agents/skills/spec-driven-development/SKILL.md) lists commands, tech stack, project structure, and code style in each specification. This project keeps them in their source documents instead:

- The [technology stack](../technology-stack.md), [project structure](../project-structure.md), and code conventions apply to all modules.
- The [constraints](../../CONSTRAINTS.md) define the standard checks. Issue verification steps name the commands for each task.
- The skill puts the test framework, test locations, coverage, and test levels in `Testing strategy`. This project keeps only the risks and their [test levels](#test-levels). The technology stack names the framework, the project structure names the locations, and the constraints set the coverage.

## Reference

### Test levels

| Level | What the test runs | Environment |
| --- | --- | --- |
| `Unit` | Code in one process. Fakes replace the database, the broker, the network, and other services. | None |
| `Integration` | Code with real dependencies, such as PostgreSQL, Redpanda, or another Flowspace service, in Docker containers that the test starts. | Docker |
| `Cluster` | The services and platform components that Tilt deploys to the local cluster, including the Bruno smoke tests. | Local cluster |

### Method requirement values

| Column | Value | Meaning |
| --- | --- | --- |
| `Authentication` | `None` | The method takes no credential. Use it only for a public method. |
| `Authentication` | `Bearer access token` | The request needs exactly one `Authorization: Bearer` header, and the subject comes from the token. |
| `Authentication` | `Service mTLS` | The caller is a service that proves its identity with a client certificate. |
| `Retry` | `Safe to retry` | A retry with the same request has no additional effect. |
| `Retry` | `Requires Idempotency-Key` | The request needs a key, and a retry with the same key returns the original result. |
| `Retry` | `Rejects Idempotency-Key` | The method rejects a key. Link the ADR that makes this exception. |
| `Retry` | `Do not retry` | A retry causes a harmful effect. Explain the effect in `Behavior`. |
