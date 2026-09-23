# Specifications

Each specification defines one capability before implementation. It records scope, observable behavior, feature-specific testing, and success criteria. An optional final section records open decisions or checks before real-user use.

| Module | Specification | Depends on | Status |
| --- | --- | --- | --- |
| `identity-signup-and-email-verification` | [Identity signup and email verification](identity-signup-and-email-verification.md) | None | Approved |
| `workspace-create-read` | [Workspace creation and reading](workspace-create-read.md) | None | Approved |

Draft specifications require approval before planning. An approved specification permits planning. An implemented specification has evidence for every success criterion.

## Specification format

All capability specifications use the core sections through `Success criteria` in this order. Add a final section for open decisions or real-user readiness when needed. Use subsections under `Required behavior` for topics that are specific to one capability.

| Section | Content |
| --- | --- |
| `Objective` | Capability, users, purpose, and success intent |
| `Scope and decision sources` | Included and excluded work, module dependencies, and relevant decisions |
| `Contract` | Consumer-visible interfaces, inputs, outputs, data, and errors |
| `Required behavior` | Business rules, invariants, security, consistency, and operational behavior |
| `Commands` | Commands that build, test, lint, or generate artifacts for this capability |
| `Testing strategy` | Feature-specific evidence and its owning test boundaries |
| `Boundaries` | Capability-specific actions to always do, ask about, or never do |
| `Success criteria` | Observable Given and Then outcomes required for completion |
| `Open questions and approval` (optional) | Unresolved decisions and the approval required for the next phase |
| `Before real users join` (optional) | Checks that must pass before real-user use |

Technology stack and code style come from shared project sources. Put implementation locations in the plan. Add a new top-level section here before using it in a specification.

## Shared project sources

Every specification inherits the sources below. A specification repeats a shared rule only when that rule changes observable behavior or completion evidence.

| Subject | Source |
| --- | --- |
| Product scope and system behavior | [Architecture](../architecture.md) |
| Accepted architecture decisions | [Architecture decision records](../adr/README.md) |
| Tools and versions | [Technology stack](../technology-stack.md) |
| Source and test locations | [Project structure](../project-structure.md) |
| Writing, Git, review, and verification | [Agent instructions](../../AGENTS.md) |
| Quality, coverage, and security limits | [Constraints](../../CONSTRAINTS.md) |

Identity specifications also inherit the [Identity threat model](../security/identity-threat-model.md). Each Identity success criterion and abuse test must reference the applicable threat IDs.

Put a project-wide rule in its source document instead of copying it into each specification.
