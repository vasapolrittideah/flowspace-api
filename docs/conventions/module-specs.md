# Module specification conventions

This convention defines the file, the format, and the status of one module specification in `docs/specs/`. A module is one capability that can be tested on its own, and its specification states what the capability does. The [specification index](../specs/README.md) lists every module. A consumer is an API client, an event consumer, or a developer who reads logs, traces, metrics, dashboards, or alerts. A shape is a name, a route, a field, or a value that a consumer can read. A condition states when an effect happens, or why. A material risk is a failure that would break a success criterion or allow a threat in a threat model.

## Template

A specification has a title, a module ID, a status line, [Objective](#objective), [Scope and ADRs](#scope-and-adrs), [Contract](#contract), [Behavior](#behavior), [Testing strategy](#testing-strategy), [Implementation boundaries](#implementation-boundaries), [Success criteria](#success-criteria), and [Assumptions and open questions](#assumptions-and-open-questions).

```markdown
# Spec: <capability name>

Module ID: `<module ID>`

Status: <current state of the specification>

## Objective

<users, the questions or results they need, and the purpose>

## Scope and ADRs

<included and excluded work and ADRs>

## Contract

<the shapes that a consumer sees: interfaces, routes, fields, events, logs, and errors>

## Behavior

<when each effect happens and why: rules, invariants, security, consistency, and operations>

## Testing strategy

<risks and the test level that proves each risk>

## Implementation boundaries

<implementer actions to always do, to ask about first, and never to do>

## Success criteria

<outcomes required for completion>

## Assumptions and open questions

<assumptions and unresolved decisions, if any>
```

### Title

- Write the capability name after the `Spec:` prefix, in sentence case.
- Use the same capability name as the link text in the `Specification` column of the index.

### Module ID

- Copy the module ID from the file name, in backticks.

### Status line

- Write one [status value](#status-values) other than `Planned`, without a final period.
- Write the same status as the `Status` column of the index.

### Objective

- Write the objective as paragraphs.
- Include one sentence about the first users and their data, such as "The first users are API clients, and all data is disposable under ADR-0022."
- Do not write assumptions here. Put them in [Assumptions and open questions](#assumptions-and-open-questions).

### Scope and ADRs

- Write the parts in this order: the included work, the excluded work, and the ADRs.
- Write the included work as one paragraph that starts with "The scope covers".
- Write the excluded work as a bullet list after the sentence "This capability excludes these items:".
- Write the ADRs as a bullet list after the sentence "These decisions apply:". Start each bullet with a link to the ADR, followed by a colon and what the ADR decides for this capability.
- If the capability has no excluded work or no ADR to list, omit that list and its sentence.
- Do not list an ADR only because it applies to every API or every event, such as the ADRs that define Protobuf, REST, versions, errors, and event delivery.

### Contract

- Include each shape that a consumer reads. Put each condition in [Behavior](#behavior) instead.
- If the contract has RPCs, start it with "Use package `flowspace.<service>.v1` and service `<Service>`." If it has events but no RPCs, start it with "Use package `flowspace.<service>.v1`."
- If a shape has fewer than two named items, such as fields, labels, or panels, write it as a paragraph directly under `## Contract`, before the subsections.
- For RPCs and events, write these tables in this order, each under a `###` heading with its name: `Methods`, `Method requirements`, `HTTP-only endpoints`, `Resource fields`, `Published events`, and `Errors`. Omit a table that has no rows.
- For each other shape, write a `###` subsection with a table that has one row for each named item. If the shape is in the [contract shapes](#contract-shapes) table, use its heading and all of its columns in that order.
- If a consumer reads a property that the contract shapes table does not list, add a column for it after the listed columns. If the table has a `Meaning` column, put the new column before `Meaning`.
- If a shape is not in the contract shapes table, name its heading with a plural noun phrase, and search the `###` headings in `docs/specs/` for the same heading. If another specification has it, copy its columns. The first specification that uses a heading sets its columns.
- If no specification has the heading, make the first column the name of the item, add one column for each property that a consumer reads, in the order that the consumer uses them, and end with a `Meaning` column when a property name does not state its meaning.
- Add a new shape to the contract shapes table only when one of its columns needs a rule that the steps above do not give, such as a fixed set of values or a required format. Add the shape to the contract shapes table in a separate PR of this convention. List that PR in [Follow-up tasks](pull-requests.md#follow-up-tasks) of the specification PR.
- Put the subsections in this order: the RPC and event tables, then the shapes in the order of the contract shapes table except [Configuration](#contract-shapes), then the other shapes in alphabetical order of their headings, and then [Configuration](#contract-shapes).
- Keep the rows of [Methods](#methods) and [Method requirements](#method-requirements) in the same RPC order.
- Do not explain the mechanics of Protobuf, REST, versions, or errors again.
- Do not add a column for rate limits, deadlines, caching, or other conditional rules. State them in [Behavior](#behavior).

#### Methods

- Write one row for each RPC.

| Column | How to write |
| --- | --- |
| `RPC` | The RPC name in backticks, such as `` `CreateWorkspace` ``. |
| `Public HTTP route` | The HTTP method and path in backticks, such as `` `POST /v1/workspaces` ``. Write path parameters in braces. Write `None (internal)` for an RPC that has no public route. |
| `Request fields` | The Protobuf field names in backticks, separated by commas. Add `(required)` after each required field. Write `None` when the request has no fields. Do not name the request message or describe credentials. |
| `Response fields` | The Protobuf field names in backticks, separated by commas. Write `None` when the response has no fields. Do not name the response message or the HTTP status. Define the fields in [Resource fields](#resource-fields) or [Behavior](#behavior). |

#### Method requirements

- Write one row for each RPC, in the order of [Methods](#methods).

| Column | How to write |
| --- | --- |
| `RPC` | The RPC name in backticks, as in [Methods](#methods). |
| `Authentication` | One [authentication value](#method-requirement-values). Add an ADR link only when the value comes from an exception or a capability-specific decision, such as `Service mTLS` under ADR-0036. |
| `Retry` | One [retry value](#method-requirement-values). Add the ADR link for `Rejects Idempotency-Key`. If the value depends on a condition, state the condition in [Behavior](#behavior). |

#### HTTP-only endpoints

- Use this table only for endpoints that are not RPCs, such as a provider callback.
- Write one row for each endpoint.

| Column | How to write |
| --- | --- |
| `Endpoint` | A short name, such as "Provider callback". |
| `Public HTTP route` | The HTTP method and path in backticks, as in [Methods](#methods). |
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
| `Fields` | The Protobuf field names in backticks, separated by commas. If the name of a field does not state its meaning, explain the field in [Behavior](#behavior). |

#### Errors

- Write one row for each condition that a client can tell apart.
- Put conditions that return the same status for a security reason in one row.

| Column | How to write |
| --- | --- |
| `Condition` | The cause as the client sees it, such as "Missing or invalid authentication". Do not name internal causes. |
| `gRPC status` | The canonical code in backticks, such as `` `InvalidArgument` ``. Write `Not applicable` when only the REST gateway returns the error. |
| `HTTP status` | The HTTP status number, such as `400`. |

### Behavior

- Write the behavior in `###` subsections. Put the subsections for the topics of the capability first, and name each with a noun phrase, such as `### Correlation`.
- After the topic subsections, add these shared subsections in this order when they apply: `### Security and abuse`, `### Data and compatibility`, and `### Diagnostics`.
- If the capability changes a schema or a stored format, state the migration, its effect on running older code, and its rollback under `### Data and compatibility`.
- Under `### Diagnostics`, list the values that logs, traces, metrics, and errors must never contain. Name the log events and fields in the [Contract](#contract) section instead of that subsection.
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

### Implementation boundaries

- Write up to three bullet lists, in this order, each after its own sentence: "Always do these actions:", "Ask the maintainer before these actions:", and "Never do these actions:". Omit a list that has no items, and its sentence.
- Start each item with a verb, such as "Change the outbox schema." Start each item of the last list with "Do not", such as "Do not read another service's database."
- Do not repeat system rules from [Behavior](#behavior).
- Do not repeat a rule from the [constraints](../../CONSTRAINTS.md) or from another convention, such as the rule that a schema change needs a new migration file.
- Do not put plan work in this section, such as a measurement to record in the plan.

### Success criteria

- Write a numbered list. Write each item as `Given <state>, When <action>, Then <result>.`, with `Given`, `When`, and `Then` capitalized.
- Refer to a criterion by its number, such as "criterion 2". Do not add IDs, such as `SC-01`.
- If the list has more than 20 items, propose a split into several modules to the maintainer before approval.
- Do not add a success criterion for the repository checks. The [constraints](../../CONSTRAINTS.md) apply to every change.

### Assumptions and open questions

- Write one bullet for each assumption or open question.
- Resolve or remove each item before approval, as the [status rules](#status-and-approval) state.

## Rules

### Naming and location

- Write the module ID in kebab-case as `<service-or-area>-<capability>`, such as `identity-provider-login` or `observability-logs-and-traces`. Do not rename an ID after it appears in the index.
- Save one specification as `docs/specs/<module-id>.md`.

### Format and content

- Use the sections through [Success criteria](#success-criteria) in the template order. Add [Assumptions and open questions](#assumptions-and-open-questions) only when the specification has an assumption or an open question.
- Put a check that a capability must pass before Flowspace stops using disposable data in the `Before real teams` list of the [architecture](../architecture.md#observability-and-recovery), not in the specification.
- Put implementation locations and commands in the plan and its Issues. Repeat a project-wide rule only when it changes observable behavior or completion criteria.
- Do not add other top-level sections. Add a new section to this convention before you use it in a specification.

### Workflow

- If a request changes an existing capability, change the specification of its module. If a request adds a capability that can be tested on its own, add a new module.
- Write the specification of a module only after the specification of each module in its `Depends on` column is `Approved` or `Implemented`. A specification can then use the approved contract of each dependency.
- If one piece of work needs several modules, add a row for each module to the index with the `Planned` status before you write the first specification. Name the dependencies of each row. The maintainer reviews this map in its own PR.

### Links and tracking

- Add a row for each module to the specification index, and keep its `Status` column the same as the status of the specification.
- Record the module dependencies only in the `Depends on` column of the index. Do not list them in the specification.
- Write the module ID in backticks in the `Module` column. In the `Specification` column, link the capability name to the file, or write the name without a link for a `Planned` module. In the `Depends on` column, write the module IDs in backticks, separated by commas, or `None`.

### Status and approval

- Let only the maintainer approve a specification. The approval is the merge of the one PR that adds the specification, or that changes its contract, behavior, or success criteria, with the `Approved` status.
- Open that PR with the `Draft` status in the specification and in the index. Use the title `docs(<scope, if any>): approve <capability> spec` for a new specification and `docs(<scope, if any>): reapprove <capability> spec` for a changed one. Write the capability name in lowercase except for names and abbreviations, such as `docs(observability): approve observability alerts and runbooks spec`.
- While the specification or the index has the `Draft` status, the `Lint Markdown` CI job fails, so GitHub blocks the merge. This failure is expected. When every other check passes, tell the maintainer that the PR is ready for review.
- When the maintainer asks for approval in the chat or in a review comment, change the status to `Approved` in the specification and in the index in one commit. Do not change it before the maintainer asks.
- Before the status changes to `Approved`, resolve each item in [Assumptions and open questions](#assumptions-and-open-questions), or remove the section.
- Start planning only from an `Approved` specification.
- If a change to an `Approved` or `Implemented` specification affects its contract, behavior, or success criteria, set its status to `Draft` in the PR of the change, and follow the approval rules above. An `Implemented` specification becomes `Approved` again, because the changed behavior is not implemented yet.
- Keep the status when a change only corrects wording or only applies a new version of this convention.
- Change the status to `Implemented` in the PR that proves the last success criterion.

### Changes

- Apply a change of this convention to new specifications, and to `Approved` specifications that a later PR changes. In that PR, convert the whole specification to the current template.
- Keep each `Implemented` specification in the format that it had when it was implemented. If a change sets its status to `Draft` again, convert the specification to the current template in the same PR.

## Differences from the spec-driven-development skill

The [`spec-driven-development` skill](../../.agents/skills/spec-driven-development/SKILL.md) gives a generic format and workflow. This convention applies where the two differ:

- Keep commands, the technology stack, the project structure, and code style out of the specification. The [technology stack](../technology-stack.md), the [project structure](../project-structure.md), the code conventions, and the [constraints](../../CONSTRAINTS.md) apply to all modules, and Issue verification steps name the commands. The skill lists them in each specification.
- Keep only the risks and their [test levels](#test-levels) in [Testing strategy](#testing-strategy). The skill also lists the test framework, the test locations, and the coverage.
- Save the specification as `docs/specs/<module-id>.md`. The skill saves it as `SPEC-<module-id>.md` at the project root.
- Use the `Planned` rows of the [specification index](../specs/README.md) as the capability map, as the [workflow rules](#workflow) state. The skill saves the map as a file at the project root.
- Write each module ID as `<service-or-area>-<capability>`. The skill uses short module IDs, such as `identity`.
- Start planning only after the approval PR merges, as the [status rules](#status-and-approval) state. The skill continues to planning after a human reviews the specification.
- Review a change to the contract, behavior, or success criteria of an `Approved` or `Implemented` specification as a new approval. The skill updates a specification when a decision or the scope changes.
- Check that the specification has the sections of the [template](#template). Do not add a section to pass the check of the skill, which looks for its six core areas.
- Name the sections `Implementation boundaries` and `Assumptions and open questions`. Write user stories as Given, When, Then outcomes in [Success criteria](#success-criteria). The skill names the sections [Boundaries and Open Questions](../../.agents/skills/spec-driven-development/SKILL.md#phase-1-specify) and puts user stories in its [Objective section](../../.agents/skills/spec-driven-development/SKILL.md#phase-1-specify).

## Reference

### Status values

| Value | Meaning |
| --- | --- |
| `Planned` | The module is in the index and has no specification file yet. |
| `Draft` | The specification is under review in an open PR. It does not reach `main` with this status. |
| `Approved` | The maintainer approved the specification, and planning can start. |
| `Implemented` | A PR proved the last success criterion. |

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
| `Retry` | `Do not retry` | A retry causes a harmful effect. Explain the effect in [Behavior](#behavior). |

### Contract shapes

| Heading | Column | How to write |
| --- | --- | --- |
| `### Log records` | `Field` | The field name in backticks, such as `` `request_id` ``. |
| `### Log records` | `Required on` | The log events that must contain the field, or `All`. |
| `### Log records` | `Meaning` | What the value is and its format. |
| `### Spans` | `Process` | The process that creates the span, such as `identity-api`. |
| `### Spans` | `Span` | The span name in backticks. |
| `### Spans` | `Kind` | The OpenTelemetry span kind in backticks, such as `` `SERVER` ``. |
| `### Spans` | `Parent` | The span or the incoming context that is the parent, or `None` for a root span. |
| `### Metrics` | `Metric` | The metric name in backticks, as Prometheus shows it. |
| `### Metrics` | `Type and unit` | The instrument type and the unit, such as "Histogram, seconds". |
| `### Metrics` | `Source` | The processes or components that emit the metric. |
| `### Metrics` | `Labels` | The label names in backticks, separated by commas, or `None`. |
| `### Dashboards` | `Dashboard` | The dashboard title. |
| `### Dashboards` | `UID` | The dashboard UID in backticks. |
| `### Dashboards` | `Panels` | The panel titles, separated by commas. |
| `### Alert rules` | `Rule title` | The `title` field of the rule. |
| `### Alert rules` | `Severity` | `page` or `ticket`. |
| `### Alert rules` | `Condition` | What the rule measures and its threshold, in words. |
| `### Alert rules` | `Pending period` | How long the condition must hold before the rule fires, such as "5 minutes". |
| `### Alert rules` | `Labels` | The labels that the rule adds other than `severity`, in backticks, or `None`. |
| `### Configuration` | `Variable` | The environment variable name in backticks. |
| `### Configuration` | `Default` | The default value in backticks, `None` when the variable is required, or `Unset` when the process runs without the variable and has no default. |
| `### Configuration` | `Meaning` | What the value controls and its allowed values. |
