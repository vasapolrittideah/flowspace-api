# Specifications

Each specification defines one capability before implementation. It records scope, observable behavior, feature-specific testing, success criteria, and open questions.

| Module | Specification | Depends on | Status |
| --- | --- | --- | --- |
| `workspace-create-read` | [Workspace creation and reading](workspace-create-read.md) | None | Draft |

Draft specifications require approval before planning. An approved specification permits planning. An implemented specification has evidence for every success criterion.

## Shared project sources

Every specification inherits the sources below. A specification repeats a shared rule only when that rule changes observable behavior or completion evidence.

| Subject | Source |
| --- | --- |
| Product scope and system behavior | [Architecture](../architecture.md) |
| Accepted architecture decisions | [Architecture decision records](../adr/README.md) |
| Tools and versions | [Technology stack](../technology-stack.md) |
| Source and test locations | [Project structure](../project-structure.md) |
| Writing, Git, review, and verification | [Contribution policy](../../CONTRIBUTING.md) |
| Quality, coverage, and security limits | [Constraints](../../CONSTRAINTS.md) |

Keep feature-specific commands, test cases, and boundaries in the owning specification. Put implementation locations in the plan. Put a project-wide rule in its source document instead of copying it into each specification.
