# Specifications

This index lists the module specifications. Follow the [Module specification](../conventions/module-specs.md) conventions for their format and status. A `Planned` row names a module that has no specification file yet, so its specification column has no link.

| Module | Specification | Depends on | Status |
| --- | --- | --- | --- |
| `identity-signup-and-email-verification` | [Identity signup and email verification](identity-signup-and-email-verification.md) | None | Implemented |
| `identity-password-login-and-sessions` | [Identity password login and sessions](identity-password-login-and-sessions.md) | `identity-signup-and-email-verification` | Implemented |
| `identity-password-recovery` | [Identity password recovery](identity-password-recovery.md) | `identity-signup-and-email-verification`, `identity-password-login-and-sessions` | Implemented |
| `identity-provider-login` | [Identity provider login](identity-provider-login.md) | `identity-signup-and-email-verification`, `identity-password-login-and-sessions` | Implemented |
| `identity-re-authentication` | [Identity re-authentication](identity-re-authentication.md) | `identity-signup-and-email-verification`, `identity-password-login-and-sessions`, `identity-provider-login` | Approved |
| `identity-session-management` | [Identity session management](identity-session-management.md) | `identity-password-login-and-sessions`, `identity-provider-login`, `identity-re-authentication` | Approved |
| `identity-password-change` | [Identity password change](identity-password-change.md) | `identity-signup-and-email-verification`, `identity-password-recovery`, `identity-re-authentication`, `identity-session-management` | Approved |
| `identity-provider-unlink` | [Identity provider unlink](identity-provider-unlink.md) | `identity-provider-login`, `identity-re-authentication`, `identity-session-management`, `identity-password-change` | Approved |
| `identity-provider-link` | [Identity provider link](identity-provider-link.md) | `identity-provider-login`, `identity-re-authentication`, `identity-provider-unlink` | Approved |
| `workspace-creation-and-reading` | [Workspace creation and reading](workspace-creation-and-reading.md) | None | Implemented |
| `observability-logs-and-traces` | [Observability logs and traces](observability-logs-and-traces.md) | None | Implemented |
| `observability-metrics-and-dashboards` | [Observability metrics and dashboards](observability-metrics-and-dashboards.md) | `observability-logs-and-traces` | Implemented |
| `observability-alerts-and-runbooks` | [Observability alerts and runbooks](observability-alerts-and-runbooks.md) | `observability-metrics-and-dashboards` | Approved |
