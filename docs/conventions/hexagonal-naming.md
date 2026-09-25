# Hexagonal naming conventions

This convention names handwritten Go components inside each service. The [project structure](../project-structure.md) defines their locations and dependency direction.

## Rules

### Do

- Name inbound adapters `<Capability>Handler`, such as `WorkspaceHandler` in `adapter/in/http/workspace_handler.go`. Handlers translate input and output at a transport boundary.
- Name application logic in `app/` `<Capability>Service`. Use the same suffix for an inbound port that exposes those operations.
- Name database adapters and their outbound ports `<Capability>Repository` when application code needs a database boundary.
- Name other outbound adapters for what they do, such as `TokenVerifier`, `IdentityClient`, `EmailSender`, or `EventPublisher`.
- Match a handwritten file name to its main component, such as `workspace_service.go`, `workspace_handler.go`, or `workspace_repository.go`.
- Keep generated Protobuf names and generated database code as produced by their tools. Apply this convention to new code and code changed for another task.

### Don't

- Do not use `Store` or `Storage` for a database component. Use `Repository` only for database access.
- Do not use `UseCase` or `Usecase` for a concrete application service or a new inbound port. Domain types can keep rules that belong to their own invariants.
- Do not put application rules in handlers or add a port for a capability that application code does not need.
- Do not rename untouched code only to satisfy this convention. A naming cleanup needs its own reviewable change.
