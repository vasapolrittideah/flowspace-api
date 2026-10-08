# Hexagonal component and file conventions

This convention names and groups the handwritten Go components and files under `services/<service>/internal/`.

The [Project structure](../project-structure.md) defines their responsibilities, locations, dependency direction, and the bootstrap composition root. [ADR-0003](../adr/0003-hexagonal-layers-inside-each-service.md) explains the Go architecture.

## Rules

### Components

- Name inbound adapters `<Capability>Handler`, including HTTP and event adapters. Do not use `<Capability>Controller`.
- Name components in `app/` `<Capability>Service`, and give their inbound ports in `port/in/` the same name. Do not use `<Capability>UseCase` or `<Capability>Usecase` for either.
- Name database adapters and their outbound ports `<Capability>Repository`. Reserve this pattern for database access. Do not use `<Capability>Store` or `<Capability>Storage`.
- Name other outbound adapters and their ports with a noun phrase that states the capability they provide, such as `TokenVerifier`.
- Use a noun phrase for each component name, such as `PasswordLoginService`.
- Name methods for their actions.
- In a file under `adapter/`, `app/`, or `port/`, define at most 1 exported `New<Capability>` constructor.
- In a file under `adapter/`, `app/`, or `port/`, match the name of the constructor to the file name, such as `NewWorkspaceService` in `workspace_service.go`.
- In a file under `domain/`, name each exported constructor `New<Concept>` after the concept that it creates.
- In `bootstrap/`, name each constructor for what it builds. Its name does not have to match the file name.
- Keep generated Protobuf names and generated database code as their tools produce them.

### Files

- Name each handwritten Go file in snake_case after its main component, capability, or domain concept, as a noun phrase, such as `password_login_service.go` or `email.go`.
- Name each bootstrap file after its server or worker role, such as `server.go`.
- Put a helper that several files of 1 package share in its own file, named after what it provides, such as `source_address.go` or `errors.go`.
- Put the interface, input, and result of an inbound port in `port/in/<capability>_service.go`, with the same file name as its service in `app/`.
- Keep the type, constructor, shared transaction code, and methods of a concrete repository in 1 file.
- When 1 handler type serves several capabilities, keep its type, constructor, and shared transport helpers in the main handler file. Put the related RPC methods and error mapping in `<capability>_handler.go`.
- Keep bootstrap wiring in the existing composition root.
- Set file boundaries separately for each layer. Do not create matching files in all 3 layers for every capability.
- Group handlers by related RPCs that serve 1 client flow, such as password recovery.
- Group services by workflows and transaction boundaries.
- Group data access by concrete repository and shared transaction mechanism.
- Keep related actions in 1 capability file, such as `email_verification_handler.go`. If no existing capability owns the work, add a file.
- Keep domain rules with the concept whose invariants they protect. Add a rule to an existing concept file when it belongs there.
- Let a domain file hold several related concepts and their constructors.
- Keep related application workflows in 1 `<capability>_service.go`. Separate them only when their outcomes or transaction boundaries define distinct capabilities.
- Keep the steps of each workflow together.
- Define an outbound port for each external capability that the application needs.
- Group the methods and transaction interfaces of an outbound port by the work that they support, not by individual queries.
- Reuse a concrete repository when it owns the same data and transaction mechanism. Add a repository when a distinct capability needs different data or a different transaction mechanism.
- Do not create a file for each method or action.
- Do not split files only by line count, RPC method, or SQL query.
- Do not add a handler type or an interface only to split files.

### Tests

- Name each test file after the production file that owns the behavior: `<name>_test.go` for unit tests and `<name>_integration_test.go` for integration tests.
- Keep the tests of 1 production file in its matching test file.
- Name a scenario test `<capability>_integration_test.go`.
- Put each scenario test in the package of the storage adapter that it uses, such as `adapter/out/postgres/`. If it uses no storage adapter, put it in the package of its inbound adapter.
- Put a test helper that several test files of 1 package use in the test file of the component that the helper sets up.

### Changes

- Apply this convention to new code and to code that another task changes. Do not rename untouched code only to satisfy this convention. A naming cleanup needs its own reviewable change.

## Examples

### Component examples

These component names follow the convention:

| Component | What it shows |
| --- | --- |
| `WorkspaceHandler` | An inbound adapter |
| `WorkspaceService` | An application component and its inbound port, with the same name |
| `WorkspaceRepository` | A database adapter and its outbound port |
| `TokenVerifier` | An outbound adapter named for its capability |

### File examples

These files follow the convention, and each path starts at `services/<service>/internal/`:

| File | What it shows |
| --- | --- |
| `adapter/in/http/workspace_handler.go` | The file of the `WorkspaceHandler` inbound adapter |
| `app/workspace_service.go` and `port/in/workspace_service.go` | The files of an application component and its inbound port, with the same file name |
| `adapter/out/postgres/workspace_repository.go` | The file of the `WorkspaceRepository` database adapter |
| `adapter/out/keycloak/token_verifier.go` | The file of an outbound adapter named for its capability |
| `adapter/in/http/identity_handler.go` and `adapter/in/http/password_recovery_handler.go` | The `IdentityHandler` type with shared code in its main file and related RPC methods in a capability file |
| `adapter/out/postgres/account_repository.go` | The `AccountRepository` that `SignupService` and `PasswordLoginService` share, because they use the same data and transaction code |
