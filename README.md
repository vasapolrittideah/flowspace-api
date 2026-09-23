# FlowSpace API

FlowSpace is a work-management platform for learning how to design distributed systems and recover from failures. This repository contains the Go backend, API contracts, and deployment configuration. The first milestone covers backend APIs and does not include a frontend.

The accepted architecture has four services that deploy independently: Identity, Workspace, Work, and Notifications. Each service owns its data. FlowSpace Identity will replace the current Keycloak integration. Workspace controls access to workspaces.

## Current status

The repository implements the first Workspace capability. The table lists the existing code and configuration. It does not establish deployment or production readiness, and the current API does not include the planned capabilities.

| Area | Current implementation | Remaining work |
| --- | --- | --- |
| Workspace | Create a workspace with an owner, read it as a member, and prevent duplicate creation on retries | Invitations, membership management, role changes, ownership transfer, and workspace archival |
| Authentication | Token validation and a local Keycloak realm with password-based token requests | Replace Keycloak with FlowSpace Identity, then add email/password and provider login, verification, recovery, and revocable sessions |
| Work | No service implementation | Projects, tasks, assignments, status changes, comments, and activity |
| Notifications | No service implementation | Event delivery, local copies of workspace access rules, and the notification inbox |
| Local runtime | k3d tasks, a Tilt configuration, PostgreSQL, and Keycloak | Repair the image build described below |
| Delivery and recovery | CI checks and local deployment manifests | Staging and production deployment, image publishing, deployment from Git, logs, metrics, traces, and tested backups |

The [Workspace Dockerfile](services/workspace/Dockerfile) leaves out the shared root `internal/` packages that both binaries need. Without these packages in the build stage, the image build fails. Tilt also excludes root `internal/` from the files that trigger image builds. Local setup cannot complete until both build inputs include the shared packages.

Treat local data as disposable. Workspace PostgreSQL uses `emptyDir`. Removing its pod deletes its data. Local Keycloak uses H2 without persistent storage configured. The project does not yet have independent backups or tested restoration.

## Prerequisites

Local development uses macOS with Docker Desktop. Before you start the local environment, install the tools below. Task runs the repository commands in `Taskfile.yaml`.

| Tool | Requirement or purpose | Installation |
| --- | --- | --- |
| Go | Version 1.27.1 from `go.mod` | [Homebrew formula](https://formulae.brew.sh/formula/go) |
| Task | Version 3 | [Homebrew formula](https://formulae.brew.sh/formula/go-task) |
| Node.js | Create local passwords and run the constraint checker and tests | [Homebrew formula](https://formulae.brew.sh/formula/node) |
| Docker Desktop | Run local containers and integration tests | [Docker installation guide](https://docs.docker.com/desktop/setup/install/mac-install/) |
| k3d | Create the local Kubernetes cluster | [Homebrew formula](https://formulae.brew.sh/formula/k3d) |
| kubectl | Select and inspect the Kubernetes cluster | [Homebrew formula](https://formulae.brew.sh/formula/kubernetes-cli) |
| Helm | Install the local Keycloak chart | [Homebrew formula](https://formulae.brew.sh/formula/helm) |
| Tilt | Build, deploy, and forward local service ports | [Homebrew formula](https://formulae.brew.sh/formula/tilt) |

If Homebrew is installed, use it to install the command-line tools:

```sh
brew install go go-task node k3d kubernetes-cli helm tilt
```

After installation, make sure that Go matches the repository version. The first setup needs network access to download tools, dependencies, images, charts, and the Tilt extension.

## Local setup

Before you start, open Docker Desktop. After cloning, run the remaining commands from the repository root. The build problem described in Current status prevents this setup from completing.

### 1. Clone the repository

```sh
git clone https://github.com/vasapolrittideah/flowspace-api.git
cd flowspace-api
go mod download
```

### 2. Install the project tools

```sh
task tools:install
```

The task installs pinned versions of Buf, sqlc, golangci-lint, actionlint, Gitleaks, and govulncheck in `bin/`. Taskfile commands call these local binaries. Run the task again after a tool version changes in `Taskfile.yaml`.

### 3. Create the local cluster

If the `flowspace` cluster already exists, run `task cluster:start` instead of creating it again. The default cluster name is `flowspace`, and its registry uses host port `5001`. Tilt requires the `k3d-flowspace` Kubernetes context.

```sh
task cluster:create
kubectl config use-context k3d-flowspace
```

### 4. Create the local password files

```sh
task secrets:setup
```

The task creates any missing local password files in `.secrets/`. Each new file contains a 64-character random hexadecimal password without a trailing newline. Running the task again keeps the existing values and sets directory permissions to `700` and file permissions to `600`. The task does not print passwords.

Tilt reads these files by default. The [Tiltfile](Tiltfile) defines the current file paths and environment variables. To use other files, set the corresponding environment variables to their paths before you start Tilt. Password files inside the repository must be in `.secrets/`. Tilt also accepts files outside the repository. Each file must contain one password without surrounding whitespace or a trailing newline.

Keep the passwords out of Git. `.gitignore` excludes `.secrets/`, and `.dockerignore` excludes it from the Docker build context. The files store passwords as plain text. Changing a password file does not change the credentials in an existing database.

Tilt reads the files and creates local Kubernetes Secrets to hold credentials for the running services. The local manifests supply the application configuration. Tilt does not load a repository `.env` file. `task secrets` scans the working directory, including `.secrets/`.

### 5. Start the local environment

After the build problem described in Current status is fixed, start Tilt:

```sh
tilt up
```

Tilt installs Keycloak and deploys Workspace PostgreSQL, the migration job, and the Workspace API. The API needs Keycloak and waits for the migration job to finish. The resources use the `flowspace-local` namespace.

| Address | Purpose |
| --- | --- |
| `http://localhost:10350` | Tilt dashboard |
| `http://localhost:8080/auth/admin/` | Keycloak administration, with username `admin` and the local administrator password |
| `http://localhost:8081` | Workspace REST and gRPC API |

If startup fails, use the Tilt dashboard or inspect the cluster resources:

```sh
kubectl --context k3d-flowspace -n flowspace-local get pods,jobs
kubectl --context k3d-flowspace -n flowspace-local logs deployment/workspace-api
kubectl --context k3d-flowspace -n flowspace-local logs job/workspace-migrate
```

Press Ctrl+C to stop Tilt. Run `task cluster:stop` to stop the local cluster. Run `task cluster:start` to start it again. If you use custom password paths, export their variables again before you start Tilt in a new shell. Do not use `task cluster:delete` to pause development. It deletes the cluster and its local data.

## API access

The current public API exposes two methods. Both require a Keycloak access token in the `Authorization: Bearer <access_token>` header. The [Protobuf service definition](contracts/proto/flowspace/workspace/v1/workspace_service.proto) defines the routes, and the [resource definition](contracts/proto/flowspace/workspace/v1/workspace.proto) defines their messages.

| Method | Route | Behavior |
| --- | --- | --- |
| `POST` | `/v1/workspaces` | Create a workspace from `{"name":"Example workspace"}` and make the authenticated subject its owner |
| `GET` | `/v1/workspaces/{workspace_id}` | Read a workspace where the authenticated subject has a membership |

Workspace creation also requires an `Idempotency-Key` header to identify one create request across retries. When you retry a creation, reuse the same key and name. Changing the name while reusing the key, or sending overlapping creation requests, returns HTTP 409. To create another workspace, use a new key.

To try the local API, create a user in the `flowspace` realm through Keycloak administration. Set a non-temporary password. Complete any required user actions. In your API client, request a token with `POST http://localhost:8080/auth/realms/flowspace/protocol/openid-connect/token` and a form-encoded body:

| Field | Value |
| --- | --- |
| `grant_type` | `password` |
| `client_id` | `workspace-api` |
| `username` | The local user's username |
| `password` | The local user's password |

Use the returned `access_token` in the authorization header for Workspace requests. The local realm allows password-based token requests for API experiments. The architecture document leaves browser-session handling open for discussion. The repository does not contain a Bruno collection.

## Development commands

Run `task --list` to see the available repository commands. Integration tests use Testcontainers and require a running Docker runtime. For the human development workflow, read [CONTRIBUTING.md](CONTRIBUTING.md). For quality requirements, read [CONSTRAINTS.md](CONSTRAINTS.md).

| Command | Purpose |
| --- | --- |
| `go test ./...` | Run Go tests without the integration build tag |
| `go test -tags=integration ./services/workspace/internal/adapter/out/postgres` | Run PostgreSQL integration tests with Docker |
| `go build ./services/workspace/cmd/...` | Compile the API and migration commands without writing binaries |
| `task fmt` | Format Go source |
| `task tools:install` | Install pinned project tools in `bin/` |
| `task check:fast` | Run constraint-checker tests, the quality floor, formatting, and secret scanning |
| `task check:task` | Run local checks, lint, coverage, and reachable dependency vulnerability scanning |
| `task coverage` | Run Go tests and enforce changed-line and total coverage requirements |
| `task vuln` | Scan reachable Go dependencies for known vulnerabilities |
| `task secrets:setup` | Create missing local passwords and set private permissions |
| `task secrets:local-test` | Test password setup, Tilt paths, and Git and Docker exclusions with Tilt and Docker |
| `task buf -- lint` | Lint Protobuf contracts |
| `task buf -- generate` | Regenerate API messages, clients, and adapters |
| `task sqlc -- generate` | Regenerate PostgreSQL query methods |

Through September 25, 2026, `task check:task` can return success even when coverage or dependency vulnerability commands fail. It reports those failures as warnings. Starting September 26, 2026, those failures block the command. Before you report that all checks pass, read each command result. Do not edit generated files by hand.

## Project documentation

The documents below explain the accepted decisions, where code belongs, and which tools the project uses. Open proposals in the architecture document remain undecided. The project structure describes the planned layout, so some directories do not exist yet.

| Document | Contents |
| --- | --- |
| [Architecture](docs/architecture.md) | Product rules, service ownership, consistency, security, environments, and open proposals |
| [Project structure](docs/project-structure.md) | Directory ownership, service layers, and dependency rules |
| [Technology stack](docs/technology-stack.md) | Selected platforms, tools, and Go packages |
| [Architecture decision records](docs/adr/README.md) | Accepted decisions, their rationale, and rejected alternatives |
| [Contributing](CONTRIBUTING.md) | Human workflow for specifications, plans, implementation, review, and launch checks |
| [Agent instructions](AGENTS.md) | Agent rules for Git, writing, verification, and pull requests |
| [Constraints](CONSTRAINTS.md) | Quality rules and their enforcement commands |

Staging and the environment named production are planned for one Ubuntu host. A failure of that host affects both environments. Deployment and recovery procedures are not implemented. The production name does not mean that the system is ready for production.
