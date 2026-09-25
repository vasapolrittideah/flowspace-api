# Hexagonal naming conventions

This convention names handwritten Go components inside each service. The [project structure](../project-structure.md) defines their locations and dependency direction.

| Responsibility | Name | Example |
| --- | --- | --- |
| Receive HTTP, RPC, or event input | `Handler` | `WorkspaceHandler` in `adapter/in/http/workspace_handler.go` |
| Coordinate application logic | `Service` | `WorkspaceService` in `app/workspace_service.go` |
| Read or write the service's database | `Repository` | `WorkspaceRepository` in `adapter/out/postgres/workspace_repository.go` |

## Rules

- Use `Repository` only for database access. Name its outbound port `<Capability>Repository` when application code needs one. Do not name a database component `Store` or `Storage`.
- Use `Service` for application logic in `app/`. Do not name a concrete application service `UseCase` or `Usecase`. Domain types can keep rules that belong to their own invariants.
- Use `Handler` for inbound adapters. A handler translates input and output at a transport boundary and calls an inbound port. It does not own application rules.
- Name an outbound adapter for what it does when it does not access the database. Examples include `TokenVerifier`, `IdentityClient`, `EmailSender`, and `EventPublisher`.
- Name an inbound port `<Capability>Service` when it exposes application operations. Do not use `UseCase` or `Usecase` in new port names.
- Name outbound ports for the capability they expose. Add a port only when application code needs that boundary. Keep generated Protobuf names and generated database code as produced by their tools.
- Match a handwritten file name to its main component, such as `workspace_service.go`, `workspace_handler.go`, or `workspace_repository.go`.
- Apply these names to new code and code changed for another task. A naming cleanup alone needs its own reviewable change.
