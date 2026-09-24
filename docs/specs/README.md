# Specifications

This index lists the module specifications. Follow the [module specification conventions](../conventions/module-specs.md) for their format, status, and shared sources.

| Module | Specification | Depends on | Status |
| --- | --- | --- | --- |
| `identity-signup-and-email-verification` | [Identity signup and email verification](identity-signup-and-email-verification.md) | None | Approved |
| `identity-password-login-and-sessions` | [Identity password login and sessions](identity-password-login-and-sessions.md) | `identity-signup-and-email-verification` | Approved |
| `identity-password-recovery` | [Identity password recovery](identity-password-recovery.md) | `identity-signup-and-email-verification`, `identity-password-login-and-sessions` | Approved |
| `identity-provider-login` | [Identity provider login](identity-provider-login.md) | `identity-signup-and-email-verification`, `identity-password-login-and-sessions` | Approved |
| `workspace-create-read` | [Workspace creation and reading](workspace-create-read.md) | None | Approved |
