# Module specification conventions

A module specification defines one capability before implementation. It records the scope, behavior, testing, and outcomes required for completion. Follow this convention when writing or updating `docs/specs/<module-id>.md`. Add each specification to the [specification index](../specs/README.md).

## Template

Use the sections through `Success criteria` in this order. Add either optional final section when the capability needs it. Add a new top-level section to this convention before using it in a specification.

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

| Part | How to write it |
| --- | --- |
| Header | Use the capability name in the title and the module ID from the file name. Use `Draft` before approval, `Approved` after approval, and `Implemented` after every success criterion has evidence. An approved specification permits planning. |
| Objective | State the capability, its users, its purpose, and the intended result. |
| Scope and decision sources | Name included and excluded work, module dependencies, and relevant accepted decisions. Put implementation locations in the plan. Repeat a project-wide rule only when it changes observable behavior or completion evidence. Put project-wide rules in their source documents. |
| Contract | Define consumer-visible interfaces, inputs, outputs, data, and errors. |
| Required behavior | Define business rules, invariants, security, consistency, and operational behavior. Use subsections for topics specific to the capability. |
| Commands | List commands that build, test, lint, or generate artifacts for the capability. |
| Testing strategy | Name feature-specific evidence and the test boundary that owns it. In Identity specifications, reference applicable [threat IDs](../security/identity-threat-model.md) in each abuse test. |
| Boundaries | Name capability-specific actions to always do, ask about, or never do. |
| Success criteria | Write observable Given and Then outcomes required for completion. In Identity specifications, reference applicable [threat IDs](../security/identity-threat-model.md) in each success criterion. |
| Open questions and approval | Include this section only when a decision remains open or approval is needed for the next phase. |
| Before real users join | Include this section only when checks must pass before real-user use. |

## Examples

The [Identity signup specification](../specs/identity-signup-and-email-verification.md) and [Workspace creation specification](../specs/workspace-create-read.md) show this format.
