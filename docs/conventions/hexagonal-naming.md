# Hexagonal naming conventions

This convention names handwritten Go components inside each service. The [project structure](../project-structure.md) defines their responsibilities, locations, and dependency direction. [ADR-0003](../adr/0003-hexagonal-layers-inside-each-service.md) explains the architecture decision.

## Rules

### Do

- Name inbound adapters `<Capability>Handler`.
- Name components in `app/` `<Capability>Service`. Use the same suffix for their inbound ports.
- Name database adapters and their outbound ports `<Capability>Repository`. Reserve this pattern for database access.
- Name other outbound adapters and ports for the capability they provide.
- Name handwritten Go files in snake_case after their main component.
- When one handler type serves several capabilities, keep its type, constructor, and shared transport helpers in the main handler file.
- Put each capability's related RPC methods and error mapping in `<capability>_handler.go`. Keep related RPCs together instead of making one file per method.
- Keep generated Protobuf names and generated database code as produced by their tools.
- Apply this convention to new code and code changed for another task.

### Don't

- Do not name an inbound adapter `<Capability>Controller`.
- Do not name a database component `<Capability>Store` or `<Capability>Storage`.
- Do not name a concrete application service or new inbound port `<Capability>UseCase` or `<Capability>Usecase`.
- Do not add a handler type or interface only to split RPC methods across files.
- Do not rename untouched code only to satisfy this convention. A naming cleanup needs its own reviewable change.

## Examples

- `WorkspaceHandler` in `adapter/in/http/workspace_handler.go` names an inbound adapter.
- `WorkspaceService` in `app/workspace_service.go` names an application component and its inbound port.
- `WorkspaceRepository` in `adapter/out/postgres/workspace_repository.go` names a database adapter and its outbound port.
- `TokenVerifier` in `adapter/out/keycloak/token_verifier.go` names an outbound adapter by its capability.
- `IdentityHandler` can keep shared code in `identity_handler.go` while related password recovery RPC methods live in `password_recovery_handler.go`.
