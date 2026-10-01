# Specifications

This index lists the module specifications. Follow the [module specification conventions](../conventions/module-specs.md) for their format and status. A `Planned` row names a module that has no specification file yet, so its specification column has no link.

| Module | Specification | Depends on | Status |
| --- | --- | --- | --- |
| `identity-signup-and-email-verification` | [Identity signup and email verification](identity-signup-and-email-verification.md) | None | Implemented |
| `identity-password-login-and-sessions` | [Identity password login and sessions](identity-password-login-and-sessions.md) | `identity-signup-and-email-verification` | Implemented |
| `identity-password-recovery` | [Identity password recovery](identity-password-recovery.md) | `identity-signup-and-email-verification`, `identity-password-login-and-sessions` | Implemented |
| `identity-provider-login` | [Identity provider login](identity-provider-login.md) | `identity-signup-and-email-verification`, `identity-password-login-and-sessions` | Implemented |
| `workspace-creation-and-reading` | [Workspace creation and reading](workspace-creation-and-reading.md) | None | Implemented |
| `observability-logs-and-traces` | Observability logs and traces | None | Planned |
| `observability-metrics-and-dashboards` | Observability metrics and dashboards | `observability-logs-and-traces` | Planned |
| `observability-alerts-and-runbooks` | Observability alerts and runbooks | `observability-metrics-and-dashboards` | Planned |
