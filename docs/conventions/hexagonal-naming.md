# Hexagonal naming conventions

This convention names handwritten Go components inside each service. The [project structure](../project-structure.md) defines their responsibilities, locations, and dependency direction. [ADR-0003](../adr/0003-hexagonal-layers-inside-each-service.md) explains the architecture decision.

## Rules

### Do

- Name inbound adapters `<Capability>Handler`.
- Name components in `app/` `<Capability>Service`. Use the same suffix for their inbound ports.
- Name database adapters and their outbound ports `<Capability>Repository`.
- Name other outbound adapters and ports for the capability they provide.
- Name handwritten Go files in snake_case after their main component.
- Keep generated Protobuf names and generated database code as produced by their tools.
- Apply this convention to new code and code changed for another task.

### Don't

- Do not use `Controller` for an inbound adapter.
- Do not use `Store` or `Storage` for a database component. Use `Repository` only for database access.
- Do not use `UseCase` or `Usecase` for a concrete application service or a new inbound port.
- Do not rename untouched code only to satisfy this convention. A naming cleanup needs its own reviewable change.

## Examples

- `WorkspaceHandler` in `adapter/in/http/workspace_handler.go` names an inbound adapter.
- `WorkspaceService` in `app/workspace_service.go` names an application component and its inbound port.
- `WorkspaceRepository` in `adapter/out/postgres/workspace_repository.go` names a database adapter and its outbound port.
- `TokenVerifier` in `adapter/out/keycloak/token_verifier.go` names an outbound adapter by its capability.
