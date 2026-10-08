# Module specification conventions

This convention defines the file, the format, and the status of 1 module specification in `docs/specs/`.

## Template

A specification has a title, a module ID, a status line, [Objective](#objective), [Scope and ADRs](#scope-and-adrs), [Contract](#contract), [Behavior](#behavior), [Testing strategy](#testing-strategy), [Implementation boundaries](#implementation-boundaries), [Success criteria](#success-criteria), and [Assumptions and open questions](#assumptions-and-open-questions).

```markdown
# Spec: <capability name>

Module ID: `<module ID>`

Status: <current state of the specification>

## Objective

<users, the questions or results they need, and the purpose>

## Scope and ADRs

The scope covers <included work>.

This capability excludes these items:

- <excluded item, if any>

These decisions apply:

- [ADR-<NNNN>](<path of the ADR file>): <what the ADR decides for this capability, if any>

## Contract

Use package `flowspace.<service name>.v1` and service `<service name in Pascal case>`.

### Methods

| RPC | Public HTTP route | Request fields | Response fields |
| --- | --- | --- | --- |
| <RPC name, if any> | <HTTP method and path> | <request fields> | <response fields> |

### Method requirements

| RPC | Authentication | Retry |
| --- | --- | --- |
| <RPC name, if any> | <authentication value> | <retry value> |

### HTTP-only endpoints

| Endpoint | Public HTTP route | Authentication | Request | Response |
| --- | --- | --- | --- | --- |
| <endpoint name, if any> | <HTTP method and path> | <authentication value> | <values that the endpoint reads> | <result of the endpoint> |

### Resource fields

<resource name, if any>:

| Protobuf field | JSON field | Meaning |
| --- | --- | --- |
| <Protobuf field name> | <JSON field name> | <meaning of the field> |

### Published events

| Event | Topic | Fields |
| --- | --- | --- |
| <event message name, if any> | <topic name> | <event fields> |

### Errors

| Condition | gRPC status | HTTP status |
| --- | --- | --- |
| <condition that a client can tell apart, if any> | <gRPC status code> | <HTTP status number> |

### <shape heading>

| <item column> | <property column> |
| --- | --- |
| <item name, if any> | <property value> |

## Behavior

### <topic of the capability>

<behavior rules>

## Testing strategy

| Risk | Test level |
| --- | --- |
| <material risk> | <test level> |

## Implementation boundaries

Always do these actions:

- <action, if any>

Ask the maintainer before these actions:

- <action, if any>

Never do these actions:

- Do not <action, if any>

## Success criteria

1. Given <state>, When <action>, Then <result>.

## Assumptions and open questions

- <assumption or open question, if any>
```

### Title

- Write the capability name after the `Spec:` prefix.
- Use the same capability name as in the `Specification` column of the index.

### Module ID

- Copy the module ID from the file name.

### Status line

- Write 1 value from [Status values](#status-values) other than `Planned`, without a final period.
- Write the same status as the `Status` column of the index.

### Objective

- Write the objective as paragraphs.
- Include 1 sentence about the first users and their data, such as "The first users are API clients, and all data is disposable under ADR-0022."
- Do not write assumptions here. Put them in [Assumptions and open questions](#assumptions-and-open-questions).

### Scope and ADRs

- Write the included work as 1 paragraph that starts with "The scope covers".
- Write the excluded work as a bullet list after the sentence "This capability excludes these items:".
- Write the ADRs as a bullet list after the sentence "These decisions apply:". Start each bullet with a link to the ADR, followed by a colon and what the ADR decides for this capability.
- If the capability has no excluded work or no ADR to list, omit that list and its sentence.
- Write the parts in this order:
  1. The included work.
  2. The excluded work.
  3. The ADRs.
- Do not list an ADR only because it applies to every API or every event, such as the ADRs that define Protobuf, REST, versions, errors, and event delivery.

### Contract

- Put each condition in [Behavior](#behavior).
- If the contract has RPCs, start it with "Use package `flowspace.<service name>.v1` and service `<service name in Pascal case>`." If it has events but no RPCs, start it with "Use package `flowspace.<service name>.v1`."
- If a shape has fewer than 2 named items, such as fields, labels, or panels, write it as a paragraph directly under `## Contract`, before the subsections.
- Write each shape that a consumer reads.
- For RPCs and events, omit a table that has no rows. Write the remaining tables in this order, each under a `###` heading with its name:
  1. [Methods](#methods)
  2. [Method requirements](#method-requirements)
  3. [HTTP-only endpoints](#http-only-endpoints)
  4. [Resource fields](#resource-fields)
  5. [Published events](#published-events)
  6. [Errors](#errors)
- For each other shape, write a `###` subsection with a table that has 1 row for each named item.
- Choose the heading and the columns of each other shape with these steps, and stop after the first match:
  1. If the shape is in the [Contract shapes](#contract-shapes) table, use its heading and all of its columns in that order.
  2. Name the heading with a plural noun phrase. Search the `###` headings in `docs/specs/` for the same heading. If another specification has it, copy its columns.
  3. Write the columns in the order of the next rule. The first specification that uses a heading sets its columns.
- For a heading that no specification has, write the columns in this order:
  1. The name of the item.
  2. A column for each property that a consumer reads, in the order that the consumer uses them.
  3. A `Meaning` column, when a property name does not state its meaning.
- If a consumer reads a property that the [Contract shapes](#contract-shapes) table does not list, add a column for it after the listed columns. If the table has a `Meaning` column, put the new column before `Meaning`.
- Put the subsections in this order:
  1. The RPC and event tables.
  2. The shapes in the order of the [Contract shapes](#contract-shapes) table, except the shape whose heading is `### Configuration`.
  3. The other shapes, in alphabetical order of their headings.
  4. The subsection whose heading is `### Configuration`.
- Keep the rows of [Methods](#methods) and [Method requirements](#method-requirements) in the same RPC order.
- Only when any column of a new shape needs a rule that the steps above do not give, such as a fixed set of values or a required format, add the shape to the [Contract shapes](#contract-shapes) table. Add it with these steps:
  1. Add the shape in a separate PR of this convention.
  2. List that PR in [Follow-up tasks](pull-requests.md#follow-up-tasks) of the specification PR.
- Do not explain the mechanics of Protobuf, REST, versions, or errors again.
- Do not add a column for rate limits, deadlines, caching, or other conditional rules. State them in [Behavior](#behavior).

#### Methods

- Write 1 row for each RPC.

| Column | How to write |
| --- | --- |
| `RPC` | The RPC name, such as `` `CreateWorkspace` ``. |
| `Public HTTP route` | The HTTP method and path, such as `` `POST /v1/workspaces` ``. Write path parameters in braces. Write `None (internal)` for an RPC that has no public route. |
| `Request fields` | The Protobuf field names, separated by commas. Add `(required)` after each required field. Write `None` when the request has no fields. Do not name the request message or describe credentials. |
| `Response fields` | The Protobuf field names, separated by commas. Write `None` when the response has no fields. Do not name the response message or the HTTP status. Define the fields in [Resource fields](#resource-fields) or [Behavior](#behavior). |

#### Method requirements

- Write 1 row for each RPC, in the order of [Methods](#methods).

| Column | How to write |
| --- | --- |
| `RPC` | The RPC name, as in [Methods](#methods). |
| `Authentication` | Choose 1 authentication value from [Method requirement values](#method-requirement-values). Use `None` only for a public method. Add an ADR link only when the value comes from an exception or a capability-specific decision, such as `Service mTLS` under ADR-0036. |
| `Retry` | Choose 1 retry value from [Method requirement values](#method-requirement-values). Add the ADR link for `Rejects Idempotency-Key`. For `Do not retry`, explain the harmful effect in [Behavior](#behavior). If the value depends on a condition, state the condition in [Behavior](#behavior). |

#### HTTP-only endpoints

- Write 1 row for each endpoint.
- Use this table only for endpoints that are not RPCs, such as a provider callback.

| Column | How to write |
| --- | --- |
| `Endpoint` | A short name, such as "Provider callback". |
| `Public HTTP route` | The HTTP method and path, as in [Methods](#methods). |
| `Authentication` | Choose 1 authentication value from [Method requirement values](#method-requirement-values). Use `None` only for a public endpoint. |
| `Request` | The query parameters, headers, or body fields that the endpoint reads. |
| `Response` | What the endpoint returns, such as a redirect or a page, and the values that it contains. |

#### Resource fields

- Write 1 table for each resource that a method returns. Put the resource name in the sentence before the table.
- Write 1 row for each field.

| Column | How to write |
| --- | --- |
| `Protobuf field` | The Protobuf field name, such as `` `created_at` ``. |
| `JSON field` | The JSON name, such as `` `createdAt` ``. |
| `Meaning` | What the value is, its format or limits, and `output only` when the server sets it. |

#### Published events

- Write 1 row for each event that the capability publishes.

| Column | How to write |
| --- | --- |
| `Event` | The Protobuf message name, such as `` `EmailDeliveryRequested` ``. |
| `Topic` | The broker topic name. |
| `Fields` | The Protobuf field names, separated by commas. If the name of a field does not state its meaning, explain the field in [Behavior](#behavior). |

#### Errors

- Write 1 row for each condition that a client can tell apart.
- Put conditions that return the same status for a security reason in 1 row.

| Column | How to write |
| --- | --- |
| `Condition` | The cause as the client sees it, such as "Missing or invalid authentication". Do not name internal causes. |
| `gRPC status` | The canonical code, such as `` `InvalidArgument` ``. Write `Not applicable` when only the REST gateway returns the error. |
| `HTTP status` | The HTTP status number, such as `400`. |

#### Log records

- Write 1 row for each log field.

| Column | How to write |
| --- | --- |
| `Field` | The field name, such as `` `request_id` ``. |
| `Required on` | The log events that must contain the field, or `All`. |
| `Meaning` | What the value is and its format. |

#### Spans

- Write 1 row for each span.

| Column | How to write |
| --- | --- |
| `Process` | The process that creates the span, such as `identity-api`. |
| `Span` | The span name. |
| `Kind` | The OpenTelemetry span kind, such as `` `SERVER` ``. |
| `Parent` | The span or the incoming context that is the parent, or `None` for a root span. |

#### Metrics

- Write 1 row for each metric.

| Column | How to write |
| --- | --- |
| `Metric` | The metric name, as Prometheus shows it. |
| `Type and unit` | The instrument type and the unit, such as "Histogram, seconds". |
| `Source` | The processes or components that emit the metric. |
| `Labels` | The label names, separated by commas, or `None`. |

#### Dashboards

- Write 1 row for each dashboard.

| Column | How to write |
| --- | --- |
| `Dashboard` | The dashboard title. |
| `UID` | The dashboard UID. |
| `Panels` | The panel titles, separated by commas. |

#### Alert rules

- Write 1 row for each alert rule.

| Column | How to write |
| --- | --- |
| `Rule title` | The `title` field of the rule. |
| `Severity` | `page` or `ticket`. |
| `Condition` | What the rule measures and its threshold, in words. |
| `Pending period` | How long the condition must hold before the rule fires, such as "5 minutes". |
| `Labels` | The labels that the rule adds other than `severity`, or `None`. |

#### Configuration

- Write 1 row for each environment variable.

| Column | How to write |
| --- | --- |
| `Variable` | The environment variable name. |
| `Default` | The default value, `None` when the variable is required, or `Unset` when the process runs without the variable and has no default. |
| `Meaning` | What the value controls and its allowed values. |

### Behavior

- Write the behavior in `###` subsections. Name each subsection for a topic of the capability with a noun phrase, such as `### Correlation`.
- Put the subsections in this order, and add each shared subsection only when it applies:
  1. The subsections for the topics of the capability.
  2. `### Security and abuse`
  3. `### Data and compatibility`
  4. `### Diagnostics`
- If the capability changes a schema or a stored format, state the migration, its effect on running older code, and its rollback under `### Data and compatibility`.
- Under `### Diagnostics`, list the values that logs, traces, metrics, and errors must never contain. Name the log events and fields in the [Contract](#contract) section instead of that subsection.
- Do not restate the 5-second request cap of [ADR-0010](../adr/0010-cap-ordinary-unary-requests-at-five-seconds.md) unless the capability has an exception.
- Do not repeat the contract.

### Testing strategy

- Write the testing strategy as the [Testing table](#testing-table).
- Do not repeat each success criterion.
- Do not list commands. Commands belong in the verification steps of each Issue, as its [Template](github-issues.md#template) shows.

#### Testing table

- Write 1 row for each material risk.

| Column | How to write |
| --- | --- |
| `Risk` | The failure that the tests must prevent, in 1 short phrase, such as "A wrong-purpose code verifies an email". In Identity specifications, add the [Identity threat model](../security/identity-threat-model.md) IDs in parentheses, such as (`ID-T03`, `ID-T19`). |
| `Test level` | Choose 1 value from [Test levels](#test-levels). If 2 levels prove the risk, write both, separated by a comma. |

### Implementation boundaries

- Write up to 3 bullet lists, each after its own sentence. Omit a list that has no items, and its sentence. Write the lists in this order:
  1. "Always do these actions:"
  2. "Ask the maintainer before these actions:"
  3. "Never do these actions:"
- Start each item with a verb, such as "Change the outbox schema." Start each item of the last list with "Do not", such as "Do not read another service's database."
- Do not repeat system rules from [Behavior](#behavior).
- Do not repeat a rule from the [Constraints](../../CONSTRAINTS.md) or from another convention, such as the rule that a schema change needs a new migration file.
- Do not put plan work in this section, such as a measurement to record in the plan.

### Success criteria

- Write 1 or more criteria in a numbered list. Capitalize `Given`, `When`, and `Then`.
- Refer to a criterion by its number, such as "criterion 2". Do not add IDs, such as `SC-01`.
- If the list has more than 20 items, propose a split into several modules to the maintainer before approval.
- Do not add a success criterion for the repository checks. The [Constraints](../../CONSTRAINTS.md) apply to every change.

### Assumptions and open questions

- Only when the specification has an assumption or an open question, add this section.
- Write 1 bullet for each assumption or open question.
- Before the status changes to `Approved`, resolve or remove each item, or remove the section.

## Rules

### Naming and location

- Write the module ID in kebab-case as `<service-or-area>-<capability>`, such as `identity-provider-login` or `observability-logs-and-traces`.
- Save 1 specification as `docs/specs/<module-id>.md`.

### Format and content

- Format the Markdown of a specification as the [Format and content](markdown-and-english-prose.md#format-and-content) rules of the Markdown and English prose conventions state.
- Use the sections in the template order.
- Put a check that a capability must pass before Flowspace stops using disposable data in the "Before real teams" list in the [Observability and recovery](../architecture.md#observability-and-recovery) section of the architecture, not in the specification.
- Put implementation locations and commands in the plan and its Issues.
- Only when a project-wide rule changes observable behavior or completion criteria, repeat the rule.
- Do not describe commands, the technology stack, the project structure, or code style. The [Technology stack](../technology-stack.md), the [Project structure](../project-structure.md), the code conventions, and the [Constraints](../../CONSTRAINTS.md) apply to all modules.
- Do not add other top-level sections. Add a new section to this convention before you use it in a specification.

### Workflow

- If a request adds a capability that can be tested on its own, add a new module.

### Links and tracking

- In the `Module` column of the index, write the module ID.
- In the `Specification` column, link the capability name to the file, or write the name without a link for a `Planned` module.
- In the `Depends on` column, write the module IDs, separated by commas, or `None`.
- Add a row for each module to the specification index, and keep its `Status` column the same as the status of the specification.
- Record the module dependencies only in the `Depends on` column of the index. Do not list them in the specification.
- If a piece of work needs several modules, map the modules with these steps:
  1. Before you write the first specification, add a row for each module to the index with the `Planned` status, and name the dependencies of each row.
  2. Open a PR with only this map, so that the maintainer reviews it.

### Status and approval

- Open the approval PR with the `Draft` status in the specification and in the index.
- Use the title `docs(<scope, if any>): approve <capability> spec` for a new specification and `docs(<scope, if any>): reapprove <capability> spec` for a changed one. Write the capability name in lowercase except for names and abbreviations, such as `docs(observability): approve observability alerts and runbooks spec`.
- While the specification or the index has the `Draft` status, expect the `Lint Markdown` CI job to fail, so that GitHub blocks the merge. When every other check passes, tell the maintainer that the PR is ready for review.
- Let only the maintainer approve a specification. The approval is the merge of the PR that adds the specification, or that changes its contract, behavior, or success criteria, with the `Approved` status.
- When the maintainer asks for approval in the chat or in a review comment, change the status to `Approved` in the specification and in the index in 1 commit. Do not change it before the maintainer asks.
- Only after the specification of each module in its `Depends on` column is `Approved` or `Implemented`, write the specification of a module. A specification can then use the approved contract of each dependency.
- Start planning only from an `Approved` specification.
- Change the status to `Implemented` in the PR that proves the last success criterion.

### Changes

- If a request changes an existing capability, change the specification of its module.
- If a change to an `Approved` or `Implemented` specification affects its contract, behavior, or success criteria, follow these steps:
  1. Set its status to `Draft` in the PR of the change.
  2. Follow the [Status and approval](#status-and-approval) rules. An `Implemented` specification becomes `Approved` again, because the changed behavior is not implemented yet.
- If a change only corrects wording or only applies a new version of this convention, keep the status.
- Apply a change of this convention only to new specifications and to specifications that a later PR changes. In that PR, convert the whole specification to the current template.
- Do not rename a module ID after it appears in the index.

## Differences from the spec-driven-development skill

This convention applies where it differs from the [Spec-Driven Development](../../.agents/skills/spec-driven-development/SKILL.md) skill:

- Follow the [Naming and location](#naming-and-location) rules for the file. The skill saves a specification as `SPEC-<module-id>.md` at the project root.
- Follow the [Naming and location](#naming-and-location) rules for the module ID. The skill uses short module IDs, such as `identity`.
- Follow the [Template](#template) for the section names. The template in [Phase 1: Specify](../../.agents/skills/spec-driven-development/SKILL.md#phase-1-specify) of the skill names the sections `## Boundaries` and `## Open Questions`.
- Follow the [Format and content](#format-and-content) rules for the sections. The check of the skill looks for its 6 core areas.
- Follow the [Format and content](#format-and-content) rules for commands, the technology stack, the project structure, and code style. The skill lists them in each specification.
- Follow the [Success criteria](#success-criteria) rules for user stories. The skill puts user stories under `## Objective`.
- Follow the [Testing strategy](#testing-strategy) rules. The skill also lists the test framework, the test locations, and the coverage.
- Follow the [Links and tracking](#links-and-tracking) rules for the capability map. The skill saves the map as a file at the project root.
- Follow the [Status and approval](#status-and-approval) rules before planning. The skill continues to planning after a human reviews the specification.
- Follow the [Changes](#changes) rules for a change to a specification. The skill updates a specification when a decision or the scope changes.

## Reference

### Status values

| Value | Meaning |
| --- | --- |
| `Planned` | The module is in the index and has no specification file yet. |
| `Draft` | The specification is under review in an open PR. |
| `Approved` | The maintainer approved the specification, and planning can start. |
| `Implemented` | A PR proved the last success criterion. |

### Test levels

| Level | What the test runs | Environment |
| --- | --- | --- |
| `Unit` | Code in 1 process. Fakes replace the database, the broker, the network, and other services. | None |
| `Integration` | Code with real dependencies, such as PostgreSQL, Redpanda, or another Flowspace service, in Docker containers that the test starts. | Docker |
| `Cluster` | The services and platform components that Tilt deploys to the local cluster, including the Bruno smoke tests. | Local cluster |

### Method requirement values

| Column | Value | Meaning |
| --- | --- | --- |
| `Authentication` | `None` | The method takes no credential. |
| `Authentication` | `Bearer access token` | The request needs exactly 1 `Authorization: Bearer` header, and the subject comes from the token. |
| `Authentication` | `Service mTLS` | The caller is a service that proves its identity with a client certificate. |
| `Retry` | `Safe to retry` | A retry with the same request has no additional effect. |
| `Retry` | `Requires Idempotency-Key` | The request needs a key, and a retry with the same key returns the original result. |
| `Retry` | `Rejects Idempotency-Key` | The method rejects a key. |
| `Retry` | `Do not retry` | A retry causes a harmful effect. |

### Contract shapes

| Heading | Columns | Rules |
| --- | --- | --- |
| `### Log records` | `Field`, `Required on`, `Meaning` | [Log records](#log-records) |
| `### Spans` | `Process`, `Span`, `Kind`, `Parent` | [Spans](#spans) |
| `### Metrics` | `Metric`, `Type and unit`, `Source`, `Labels` | [Metrics](#metrics) |
| `### Dashboards` | `Dashboard`, `UID`, `Panels` | [Dashboards](#dashboards) |
| `### Alert rules` | `Rule title`, `Severity`, `Condition`, `Pending period`, `Labels` | [Alert rules](#alert-rules) |
| `### Configuration` | `Variable`, `Default`, `Meaning` | [Configuration](#configuration) |
