# Module specification conventions

This convention defines the format, status, and approval of one module's capability specification.

## Template

A specification has the following fields and sections:

```markdown
# Spec: <capability name>

Module id: `<module-id>`

Status: Draft

## Objective

<users, the questions or results they need, and the purpose>

## Scope and decisions

<included and excluded work, module dependencies, and ADRs>

## Contract

<consumer-visible interfaces, fields, data, and errors>

## Behavior

<rules, invariants, security, consistency, and operations>

## Testing strategy

<risks, the test level that proves each risk, and the environment that each level needs>

## Implementation boundaries

<implementer actions to always do, ask about, or never do>

## Success criteria

<numbered Given, When, Then outcomes>

## Assumptions and open questions

<assumptions and unresolved decisions, if any>

## Readiness for real teams

<checks that this capability must pass before Flowspace stops using disposable data, if any>
```

- Header: The capability name, the module ID, and the status.
- Objective: The users, the questions or results that they need, and the purpose.
- Scope and decisions: Included and excluded work, module dependencies, and the ADRs that apply.
- Contract: The shape that a consumer sees.
- Behavior: When each effect happens and why.
- Testing strategy: The risks, the test level that proves each risk, and the environment that each level needs.
- Implementation boundaries: Actions that limit the implementer.
- Success criteria: Numbered Given, When, Then outcomes required for completion.
- Assumptions and open questions: Assumptions and decisions that are not yet made.
- Readiness for real teams: Checks that the capability must pass before Flowspace stops using disposable data.

The [section forms](#section-forms) state how to write each section.

## Section forms

Each subsection states what a section contains and the form in which to write it.

### Objective

- Name the users and the questions or results that they need, and state the purpose of the capability.
- Include one sentence about the first users and their data, such as "The first users are API clients, and all data is disposable under ADR-0022."
- Put assumptions in `Assumptions and open questions`.

### Scope and decisions

- Start with `Depends on: <module-id>, <module-id>.` or `Depends on: none.` Name each module as the `Depends on` column of the index lists it.
- Name the included work. Then write excluded work as a bullet list after "This capability excludes these items:".
- Write each decision as a bullet that starts with a link to the ADR, followed by a colon and what the ADR decides for this capability.

### Contract

Define the shape that a consumer sees: interfaces, routes, fields, data, and error codes. A consumer can be a developer who reads logs, not only an API client.

For an API specification, start the contract with "Use package `flowspace.<service>.v1` and service `<Service>`." List the ADRs that define Protobuf, REST, versions, and errors in `Scope and decisions`, and do not explain their mechanics again.

Use these tables in this order: `Methods`, `Method requirements`, `HTTP-only endpoints`, `Resource fields`, and `Errors`. Omit a table that has no rows. Keep the rows of `Methods` and `Method requirements` in the same RPC order.

Do not add a column for rate limits, deadlines, caching, or other conditional rules. State them in `Behavior`.

#### Methods

Write one row for each RPC.

| Column | How to write |
| --- | --- |
| `RPC` | The RPC name in backticks, such as `` `CreateWorkspace` ``. |
| `Public HTTP route` | The HTTP method and path in backticks, such as `` `POST /v1/workspaces` ``. Write path parameters in braces. Write `None (internal)` for an RPC that has no public route. |
| `Request fields` | The Protobuf field names in backticks, separated by commas. Add `(required)` after each required field. Write `None` when the request has no fields. Do not name the request message or describe credentials. |
| `Response fields` | The Protobuf field names in backticks, separated by commas. Write `None` when the response has no fields. Do not name the response message or the HTTP status. Define the fields in `Resource fields` or `Behavior`. |

#### Method requirements

Write one row for each RPC, in the order of `Methods`.

| Column | How to write |
| --- | --- |
| `RPC` | The RPC name in backticks, as in `Methods`. |
| `Authentication` | One [authentication value](#method-requirement-values). Add an ADR link only when the value comes from an exception or a capability-specific decision, such as `Service mTLS` under ADR-0036. |
| `Retry` | One [retry value](#method-requirement-values). Add the ADR link for `Rejects Idempotency-Key`. If the value depends on a condition, state the condition in `Behavior`. |

#### HTTP-only endpoints

Use this table only for endpoints that are not RPCs, such as a provider callback. Write one row for each endpoint.

| Column | How to write |
| --- | --- |
| `Endpoint` | A short name, such as "Provider callback". |
| `Public HTTP route` | The HTTP method and path in backticks, as in `Methods`. |
| `Authentication` | One [authentication value](#method-requirement-values). |
| `Request` | The query parameters, headers, or body fields that the endpoint reads, in backticks. |
| `Response` | What the endpoint returns, such as a redirect or a page, and the values that it contains. |

#### Resource fields

Write one table for each resource that a method returns, and put the resource name in the sentence before the table. Write one row for each field.

| Column | How to write |
| --- | --- |
| `Protobuf field` | The Protobuf field name in backticks, such as `` `created_at` ``. |
| `JSON field` | The JSON name in backticks, such as `` `createdAt` ``. |
| `Meaning` | What the value is, its format or limits, and `output only` when the server sets it. |

#### Errors

Write one row for each condition that a client can tell apart. Put conditions that return the same status for a security reason in one row.

| Column | How to write |
| --- | --- |
| `Condition` | The cause as the client sees it, such as "Missing or invalid authentication". Do not name internal causes. |
| `gRPC status` | The canonical code in backticks, such as `` `InvalidArgument` ``. Write `Not applicable` when only the REST gateway returns the error. |
| `HTTP status` | The HTTP status number, such as `400`. |

### Behavior

- State when each effect happens and why: business rules, invariants, security, consistency, and operations. Do not repeat the contract.
- Put subsections for the topics of the capability first. Then add these shared subsections in this order when they apply: `### Security and abuse`, `### Data and compatibility`, and `### Diagnostics`.
- If the capability changes a schema or a stored format, state the migration, its effect on running older code, and its rollback under `### Data and compatibility`.
- Under `### Diagnostics`, name the events or fields that the capability records. Then list the values that logs, traces, metrics, and errors must never contain.
- Do not restate the five-second request cap of [ADR-0010](../adr/0010-cap-ordinary-unary-requests-at-five-seconds.md) unless the capability has an exception.

### Testing strategy

Write the testing strategy as the [testing table](#testing-table). Do not repeat each success criterion or list commands. Commands belong in the verification steps of each [Issue](github-issues.md#template).

#### Testing table

Write one row for each material risk.

| Column | How to write |
| --- | --- |
| `Risk` | The failure that the tests must prevent, in one short phrase, such as "A wrong-purpose code verifies an email". In Identity specifications, add the [threat IDs](../security/identity-threat-model.md) in parentheses, such as "(ID-T03, ID-T19)". |
| `Test level` | One of `Unit`, `Integration`, `Cross-service`, or `Cluster`. If two levels prove the risk, write both, separated by a comma. |
| `Environment` | What the test level needs to run, such as `None`, `Docker`, or `Local cluster`. Do not write commands. |

### Implementation boundaries

- Name actions that limit the implementer under `### Always`, `### Ask first`, and `### Never`. Examples are "Ask first before you change the outbox schema" and "Never read another service's database".
- Start each item with a verb. Start each `Never` item with "Do not".
- Do not repeat system rules from `Behavior`.

### Success criteria

- Write a numbered list of observable Given, When, Then outcomes required for completion.
- If a specification has more than about 20 success criteria, consider a split into several modules.
- Do not add a success criterion for the repository checks. The [constraints](../../CONSTRAINTS.md) apply to every change.

### Assumptions and open questions

Write the assumptions that the specification makes and the decisions that are not yet made. The [status rules](#status-and-approval) state when to resolve them.

### Readiness for real teams

Write the checks that this capability must pass before Flowspace stops using disposable data under [ADR-0022](../adr/0022-single-host-storage-holds-disposable-data.md) and the [architecture](../architecture.md#observability-and-recovery). List only checks that belong to this capability, and link the architecture for checks that apply to the whole system.

## Rules

### Modules and files

- Write the capability name in the title and the module ID from the file name in the header.
- A module is one capability that can be tested on its own. Write its ID in kebab-case as `<service-or-area>-<capability>`, such as `identity-provider-login` or `observability-logs-and-traces`. Do not rename an ID after it appears in the index.
- Save one specification as `docs/specs/<module-id>.md`, and add each module to the [specification index](../specs/README.md).
- If one piece of work needs several modules, add a row for each module to the index with the `Planned` status before you write the first specification. Name the dependencies of each row. The maintainer reviews this map in its own PR.

### Sections

- Use the sections through `Success criteria` in the template order.
- Add either optional final section only when the capability needs it. Add a new top-level section to this convention before you use it in a specification.
- Put implementation locations and commands in the plan and its Issues. Repeat a project-wide rule only when it changes observable behavior or completion criteria. Put project-wide rules in their source documents.

### Status and approval

- The status is `Planned`, `Draft`, `Approved`, or `Implemented`, and it matches the status in the [specification index](../specs/README.md).
- `Planned` means that the module is in the index and has no specification file yet. `Draft` means that the specification exists and is not approved. A `Draft` specification can merge into `main`, but it does not permit planning.
- Only the maintainer approves a specification. The approval is the merge of a PR that changes the status to `Approved` in the specification and in the index in the same commit. Use the PR title `docs(<scope>): approve <capability> spec`. An agent changes the status to `Approved` only when the maintainer asks for it in chat or in the PR review.
- Before the status changes to `Approved`, resolve each item in `Assumptions and open questions`, or remove the section.
- An approved specification permits planning.
- If a change to an approved or implemented specification affects its contract, behavior, or success criteria, change its status to `Draft` in the same PR. The specification needs a new approval PR. A change that only corrects wording keeps the status.
- Change the status to `Implemented` in the PR that proves the last success criterion.
- Keep each `Implemented` specification in the format that it had when it was implemented.

## Differences from the spec-driven-development skill

The [`spec-driven-development` skill](../../.agents/skills/spec-driven-development/SKILL.md) lists commands, tech stack, project structure, and code style in each specification. This project keeps them in their source documents instead:

- The [technology stack](../technology-stack.md), [project structure](../project-structure.md), and code conventions apply to all modules.
- The [constraints](../../CONSTRAINTS.md) define the standard checks. Issue verification steps name the commands for each task.

## Reference

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
