# Hexagonal naming and file conventions

This convention names and groups handwritten Go components and SQL files inside each service. The [project structure](../project-structure.md) defines their responsibilities, locations, and dependency direction. [ADR-0003](../adr/0003-hexagonal-layers-inside-each-service.md) explains the Go architecture. [ADR-0014](../adr/0014-relational-data-uses-explicit-sql.md) explains the SQL decision.

## Rules

### File naming

- Name inbound adapters `<Capability>Handler`. Do not use `<Capability>Controller`.
- Name components in `app/` `<Capability>Service`. Use the same suffix for their inbound ports. Do not use `<Capability>UseCase` or `<Capability>Usecase` for either.
- Name database adapters and their outbound ports `<Capability>Repository`. Reserve this pattern for database access. Do not use `<Capability>Store` or `<Capability>Storage`.
- Name other outbound adapters and ports for the capability they provide.
- Name handwritten Go files in snake_case after their main component or capability.
- Use a noun phrase for each component and file name. Examples are `PasswordLoginService` in `password_login_service.go` and `ClaimCodeService` in `claim_code_service.go`. Name methods for their actions.
- Keep related actions in one capability file, such as `email_verification_handler.go` or `workspace_service.go`. If no existing capability owns the work, add a file. Do not create a file for each method or action.
- Keep generated Protobuf names and generated database code as produced by their tools.
- Apply this convention to new code and code changed for another task. Do not rename untouched code only to satisfy this convention. A naming cleanup needs its own reviewable change.

### File boundaries

A capability is a task that the service performs. A transaction groups database changes that must commit or roll back together.

- Group files by the capability and its transaction boundary. Do not split files only by line count, RPC method, or SQL query.
- Keep domain rules with the concept whose invariants they protect. Add a rule to an existing concept file when it belongs there.
- Keep related application workflows in one `<capability>_service.go`. Separate them only when their outcomes or transaction boundaries define distinct capabilities. Keep the steps of each workflow together.
- Keep an inbound port's interface, input, and result in the file for its capability.
- Define an outbound port for an external capability that the application needs. Group methods and transaction interfaces by the work they support, rather than by individual queries.
- Reuse a concrete repository when it owns the same data and transaction mechanism. Keep its type, constructor, shared transaction code, and related methods in the capability file. Separate methods only when they support distinct capabilities.
- When one handler type serves several capabilities, keep its type, constructor, and shared transport helpers in the main handler file. Put related RPC methods and error mapping in `<capability>_handler.go`. Do not add a handler type or interface only to split files.
- Keep bootstrap wiring in the existing composition root.
- Group test files by the behavior and transaction they verify. Name each file for the behavior it tests. Keep shared test setup together, and do not require one test file per production file.

### SQL queries

- Group related queries by capability or transaction in a snake_case file under the owning service's `db/queries/`, such as `password_recovery.sql`.
- Keep queries used by several capabilities in a common file, such as `identity.sql`. Split the file when a distinct capability has a group of related queries that changes independently. Do not split by line count, table, RPC method, or individual query.
- Keep query names unique within the service's `db/queries/` directory so sqlc can generate the Go methods.

### SQL migrations

- Put schema changes that must deploy together in one numbered file under the owning service's `db/migrations/`. Include related tables, indexes, constraints, and data changes in that file. Put independent changes in separate files.
- Name new files `<number>_<verb>_<object>.sql` in snake_case. Use a verb that describes the change, such as `00002_add_workspace_creation_expiry.sql`.
- Once a migration runs in a shared environment, leave its file unchanged. Add a new numbered migration for later changes.

## Examples

- `WorkspaceHandler` in `adapter/in/http/workspace_handler.go` names an inbound adapter.
- `WorkspaceService` in `app/workspace_service.go` names an application component and its inbound port.
- `WorkspaceRepository` in `adapter/out/postgres/workspace_repository.go` names a database adapter and its outbound port.
- `TokenVerifier` in `adapter/out/keycloak/token_verifier.go` names an outbound adapter by its capability.
- `IdentityHandler` can keep shared code in `identity_handler.go` while related password recovery RPC methods live in `password_recovery_handler.go`.
