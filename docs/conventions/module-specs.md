# Module specification conventions

This convention defines the format, status, and approval of one module's capability specification.

## Template

A specification has the following fields and sections:

```markdown
# Spec: <capability name>

Module id: `<module-id>`

Status: Draft.

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

- Header: The capability name in the title and the module ID from the file name. The status is `Planned`, `Draft`, `Approved`, or `Implemented`, and it matches the status in the [specification index](../specs/README.md).
- Objective: Name the users and the questions or results they need, and state the purpose of the capability. Put assumptions in `Assumptions and open questions`.
- Scope and decisions: Name included and excluded work. Name each module dependency by its module ID, as the `Depends on` column of the index lists it. Link each ADR that the capability applies.
- Contract: Define the shape that a consumer sees: interfaces, routes, fields, data, and error codes. A consumer can be a developer who reads logs, not only an API client. Put the table of error codes here.
- Behavior: State when each effect happens and why: business rules, invariants, security, consistency, and operations. Do not repeat the contract.
- Testing strategy: Name each material risk, the test level that proves it, and the environment that the level needs. Test levels include unit, integration, cross-service, and cluster tests. Do not repeat each success criterion or list commands. Commands belong in the verification steps of each [Issue](github-issues.md#template).
- Implementation boundaries: Name actions that limit the implementer under `### Always`, `### Ask first`, and `### Never`. Examples are "Ask first before you change the outbox schema" and "Never read another service's database". Do not repeat system rules from `Behavior`.
- Success criteria: A numbered list of observable Given, When, Then outcomes required for completion.
- Assumptions and open questions: Assumptions that the specification makes and decisions that are not yet made.
- Readiness for real teams: Checks that this capability must pass before Flowspace stops using disposable data under [ADR-0022](../adr/0022-single-host-storage-holds-disposable-data.md) and the [architecture](../architecture.md#observability-and-recovery). List only checks that belong to this capability, and link the architecture for checks that apply to the whole system.

## Rules

### Modules and files

- A module is one capability that can be tested on its own. Write its ID in kebab-case as `<service-or-area>-<capability>`, such as `identity-provider-login` or `observability-logs-and-traces`. Do not rename an ID after it appears in the index.
- Save one specification as `docs/specs/<module-id>.md`, and add each module to the [specification index](../specs/README.md).
- If one piece of work needs several modules, add a row for each module to the index with the `Planned` status before you write the first specification. Name the dependencies of each row. The maintainer reviews this map in its own PR.
- If a specification has more than about 20 success criteria, consider a split into several modules.

### Sections

- Use the sections through `Success criteria` in the template order.
- Add either optional final section only when the capability needs it. Add a new top-level section to this convention before you use it in a specification.
- Use subsections under `Behavior` for topics of the capability. Consider security, data and compatibility, and diagnostics. If the capability changes a schema or a stored format, state the migration, its effect on running older code, and its rollback under data and compatibility.
- In Identity specifications, map abuse tests in the Testing strategy to applicable [threat IDs](../security/identity-threat-model.md).
- Put implementation locations and commands in the plan and its Issues. Repeat a project-wide rule only when it changes observable behavior or completion criteria. Put project-wide rules in their source documents.
- Do not add a success criterion for the repository checks. The [constraints](../../CONSTRAINTS.md) apply to every change.

### Status and approval

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
