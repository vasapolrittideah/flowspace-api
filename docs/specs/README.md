# Specifications

This index lists the module specifications and their shared project sources. Follow the [module specification conventions](../conventions/module-specs.md) for their format and status.

| Module | Specification | Depends on | Status |
| --- | --- | --- | --- |
| `identity-signup-and-email-verification` | [Identity signup and email verification](identity-signup-and-email-verification.md) | None | Approved |
| `identity-password-login-and-sessions` | [Identity password login and sessions](identity-password-login-and-sessions.md) | `identity-signup-and-email-verification` | Approved |
| `identity-password-recovery` | [Identity password recovery](identity-password-recovery.md) | `identity-signup-and-email-verification`, `identity-password-login-and-sessions` | Approved |
| `identity-provider-login` | [Identity provider login](identity-provider-login.md) | `identity-signup-and-email-verification`, `identity-password-login-and-sessions` | Approved |
| `workspace-create-read` | [Workspace creation and reading](workspace-create-read.md) | None | Approved |

## Shared project sources

Every specification inherits the sources below.

| Subject | Source |
| --- | --- |
| Product scope and system behavior | [Architecture](../architecture.md) |
| Accepted architecture decisions | [Architecture decision records](../adr/README.md) |
| Tools and versions | [Technology stack](../technology-stack.md) |
| Source and test locations | [Project structure](../project-structure.md) |
| Writing, Git, review, and verification | [Agent instructions](../../AGENTS.md) |
| Quality, coverage, and security limits | [Constraints](../../CONSTRAINTS.md) |

Identity specifications also inherit the [Identity threat model](../security/identity-threat-model.md).
