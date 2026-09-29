# Hexagonal naming and file conventions

This convention names and groups handwritten Go components inside each service. The [project structure](../project-structure.md) defines their responsibilities, locations, and dependency direction. [ADR-0003](../adr/0003-hexagonal-layers-inside-each-service.md) explains the architecture decision.

## Rules

### File naming

- Name inbound adapters `<Capability>Handler`. Do not use `<Capability>Controller`.
- Name components in `app/` `<Capability>Service`. Use the same suffix for their inbound ports. Do not use `<Capability>UseCase` or `<Capability>Usecase` for either.
- Name database adapters and their outbound ports `<Capability>Repository`. Reserve this pattern for database access. Do not use `<Capability>Store` or `<Capability>Storage`.
- Name other outbound adapters and ports for the capability they provide.
- Name handwritten Go files in snake_case after their main component or capability.
- Keep generated Protobuf names and generated database code as produced by their tools.
- Apply this convention to new code and code changed for another task. Do not rename untouched code only to satisfy this convention. A naming cleanup needs its own reviewable change.

### File boundaries

A capability is a task that the service performs. A transaction groups database changes that must commit or roll back together.

- Group files by the capability and its transaction boundary. Do not split files only by line count, RPC method, or SQL query.
- Keep domain rules with the concept whose invariants they protect. Add a rule to an existing concept file when it belongs there.
- Put a distinct application workflow in its own `<capability>_service.go` when it has a separate outcome or transaction boundary. Keep the steps of one workflow together.
- Keep an inbound port's interface, input, and result together in the file that matches its application service.
- Define an outbound port for an external capability that the application needs. Group methods and transaction interfaces by the work they support, rather than by individual queries.
- Reuse a concrete repository when it owns the same data and transaction mechanism. Keep its type, constructor, and shared transaction code together. Put capability-specific methods in separate files when their workflows differ.
- When one handler type serves several capabilities, keep its type, constructor, and shared transport helpers in the main handler file. Put related RPC methods and error mapping in `<capability>_handler.go`. Do not add a handler type or interface only to split files.
- Keep bootstrap wiring in the existing composition root. Keep tests beside the behavior they verify, with file names based on that behavior.

## Examples

- `WorkspaceHandler` in `adapter/in/http/workspace_handler.go` names an inbound adapter.
- `WorkspaceService` in `app/workspace_service.go` names an application component and its inbound port.
- `WorkspaceRepository` in `adapter/out/postgres/workspace_repository.go` names a database adapter and its outbound port.
- `TokenVerifier` in `adapter/out/keycloak/token_verifier.go` names an outbound adapter by its capability.
- `IdentityHandler` can keep shared code in `identity_handler.go` while related password recovery RPC methods live in `password_recovery_handler.go`.
