# Module specification conventions

This convention defines the format for one module's capability specification.

## Template

A specification has the following fields and sections:

```text
# Spec: <capability name>

Module id: `<module-id>`

Status: Draft.

## Objective

<capability, users, purpose, and success intent>

## Scope and decision sources

<included and excluded work, dependencies, and decisions>

## Contract

<interfaces, inputs, outputs, data, and errors>

## Required behavior

<rules, invariants, security, consistency, and operations>

## Commands

<build, test, lint, and generation commands>

## Testing strategy

<feature-specific evidence and test ownership>

## Boundaries

<actions to always do, ask about, or never do>

## Success criteria

<observable Given and Then outcomes>

## Open questions and approval

<unresolved decisions and required approval, if any>

## Before real users join

<readiness checks, if any>
```

| Part | Content and format |
| --- | --- |
| Header | The capability name in the title and the module ID from the file name. The status is `Draft`, `Approved`, or `Implemented`. |
| Objective | State the capability, its users, its purpose, and the intended result. |
| Scope and decision sources | Included and excluded work, module dependencies, and relevant accepted decisions. |
| Contract | Define consumer-visible interfaces, inputs, outputs, data, and errors. |
| Required behavior | Business rules, invariants, security, consistency, and operational behavior. |
| Commands | List commands that build, test, lint, or generate artifacts for the capability. |
| Testing strategy | Feature-specific evidence and the test boundary that owns it. |
| Boundaries | Name capability-specific actions to always do, ask about, or never do. |
| Success criteria | Observable Given and Then outcomes required for completion. |
| Open questions and approval | Unresolved decisions and required approval, when applicable. |
| Before real users join | Readiness checks, when applicable. |

## Rules

- Follow this convention when writing or updating `docs/specs/<module-id>.md`.
- Save one specification as `docs/specs/<module-id>.md`. Use the sections through `Success criteria` in the template order.
- Add either optional final section only when the capability needs it. Add a new top-level section to this convention before using it in a specification. Add each specification to the [specification index](../specs/README.md).
- Use `Draft` before approval, `Approved` after approval, and `Implemented` after every success criterion has evidence. An approved specification permits planning.
- Put implementation locations in the plan. Repeat a project-wide rule only when it changes observable behavior or completion evidence. Put project-wide rules in their source documents.
- Use subsections under `Required behavior` for topics specific to the capability.
- In Identity specifications, reference applicable [threat IDs](../security/identity-threat-model.md) in each abuse test and success criterion.

## Examples

The [Identity signup specification](../specs/identity-signup-and-email-verification.md) and [Workspace creation specification](../specs/workspace-create-read.md) show this format.
