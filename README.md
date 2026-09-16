# FlowSpace API

FlowSpace is a work-management platform built to learn distributed-system design and failure recovery. This repository contains the Go backend, API contracts, and deployment configuration. The first milestone covers backend APIs without a frontend.

The target architecture has three independently deployable services: Workspace, Work, and Notifications. Each service owns its data. Keycloak owns authentication, and FlowSpace owns workspace access rules.

## Current status

The repository currently implements the first Workspace capability. The table describes existing code and configuration, not deployment or production readiness. Planned capabilities remain outside the current API.

| Area | Current implementation | Remaining work |
| --- | --- | --- |
| Workspace | Create a workspace with an owner, read it as a member, and prevent duplicate creation on retries | Invitations, membership management, role changes, ownership transfer, and workspace archival |
| Authentication | Token validation and a local Keycloak realm with password-based token requests | Google login, email verification and recovery integration, and durable identity storage |
| Work | No service implementation | Projects, tasks, assignments, status changes, comments, and activity |
| Notifications | No service implementation | Event delivery, local copies of workspace access rules, and the notification inbox |
| Local runtime | k3d tasks, a Tilt configuration, PostgreSQL, and Keycloak | Repair the image build described below |
| Delivery and recovery | CI checks and local deployment manifests | Staging and production deployment, image publishing, deployment from Git, logs, metrics, traces, and tested backups |

The current [Workspace Dockerfile](services/workspace/Dockerfile) omits the shared root `internal/` packages required by both binaries. The image build fails because those packages are absent from its build stage. Tilt also excludes root `internal/` from the files that trigger image builds. The local startup sequence below cannot complete until these build inputs include the shared packages.

Treat local data as disposable. Workspace PostgreSQL uses `emptyDir`, which loses data when its pod is removed. Local Keycloak uses H2 without configured persistent storage. Independent backups and tested restoration are not implemented.

## Prerequisites

Local development uses macOS with Docker Desktop. Install the tools below before starting the local environment. Task runs repository commands defined in `Taskfile.yaml`.

| Tool | Requirement or purpose | Installation |
| --- | --- | --- |
| Go | Version 1.27.1 from `go.mod` | [Homebrew formula](https://formulae.brew.sh/formula/go) |
| Task | Version 3 | [Homebrew formula](https://formulae.brew.sh/formula/go-task) |
| Node.js | Run the constraint checker and its built-in tests | [Homebrew formula](https://formulae.brew.sh/formula/node) |
| golangci-lint | Version 2.13.2, matching CI | [Homebrew formula](https://formulae.brew.sh/formula/golangci-lint) |
| Docker Desktop | Run local containers and integration tests | [Docker installation guide](https://docs.docker.com/desktop/setup/install/mac-install/) |
| k3d | Create the local Kubernetes cluster | [Homebrew formula](https://formulae.brew.sh/formula/k3d) |
| kubectl | Select and inspect the Kubernetes cluster | [Homebrew formula](https://formulae.brew.sh/formula/kubernetes-cli) |
| Helm | Install the local Keycloak chart | [Homebrew formula](https://formulae.brew.sh/formula/helm) |
| Tilt | Build, deploy, and forward local service ports | [Homebrew formula](https://formulae.brew.sh/formula/tilt) |

With Homebrew installed, install the command-line tools:

```sh
brew install go go-task node golangci-lint k3d kubernetes-cli helm tilt
```

Make sure that Go and golangci-lint match the repository versions after installation. Buf, sqlc, Gitleaks, and govulncheck run through pinned Go commands in `Taskfile.yaml`. You do not need separate installations for those tools. The first run requires network access to download dependencies, images, charts, and the Tilt extension.

## Local setup

Start Docker Desktop before running the commands in this section. Run repository commands from the repository root. The image-build limitation in Current status applies to this sequence.

Clone the repository:

```sh
git clone https://github.com/vasapolrittideah/flowspace-api.git
cd flowspace-api
go mod download
```

Create the local cluster:

```sh
task cluster:create
kubectl config use-context k3d-flowspace
```

If the `flowspace` cluster already exists, run `task cluster:start` instead of creating it again. The default cluster name is `flowspace`, and its registry uses host port `5001`. Tilt requires the `k3d-flowspace` Kubernetes context.

Prepare two password files outside the repository. Each file must contain one password without surrounding whitespace or a trailing newline. The commands below create random passwords only when the files do not exist:

```sh
umask 077
mkdir -p "$HOME/.local/share/flowspace/local"
export KEYCLOAK_ADMIN_PASSWORD_FILE="$HOME/.local/share/flowspace/local/keycloak-admin-password"
export WORKSPACE_DATABASE_PASSWORD_FILE="$HOME/.local/share/flowspace/local/workspace-database-password"
test -f "$KEYCLOAK_ADMIN_PASSWORD_FILE" || openssl rand -hex 32 | tr -d '\n' > "$KEYCLOAK_ADMIN_PASSWORD_FILE"
test -f "$WORKSPACE_DATABASE_PASSWORD_FILE" || openssl rand -hex 32 | tr -d '\n' > "$WORKSPACE_DATABASE_PASSWORD_FILE"
```

Keep the passwords outside Git. Tilt reads the files and creates local Kubernetes Secrets, which hold credentials for the running services. The application receives configuration from the local manifests. Tilt does not load a repository `.env` file.

After the image-build limitation is resolved, start the local environment:

```sh
tilt up
```

Tilt installs Keycloak and deploys Workspace PostgreSQL, the migration job, and the Workspace API. The API depends on Keycloak and completion of the migration job. The resources use the `flowspace-local` namespace.

| Address | Purpose |
| --- | --- |
| `http://localhost:10350` | Tilt dashboard |
| `http://localhost:8080/auth/admin/` | Keycloak administration, with username `admin` and the password from `KEYCLOAK_ADMIN_PASSWORD_FILE` |
| `http://localhost:8081` | Workspace REST and gRPC API |

To inspect startup failures, use the Tilt dashboard or inspect the cluster resources:

```sh
kubectl --context k3d-flowspace -n flowspace-local get pods,jobs
kubectl --context k3d-flowspace -n flowspace-local logs deployment/workspace-api
kubectl --context k3d-flowspace -n flowspace-local logs job/workspace-migrate
```

Press Ctrl+C to stop Tilt. Run `task cluster:stop` to stop the local cluster and `task cluster:start` to start it again. Export both password-file variables again before starting Tilt in a new shell. Do not use `task cluster:delete` to pause development because it deletes the cluster and its local data.

## API access

The current public API exposes two methods. Both require a Keycloak access token in the `Authorization: Bearer <access_token>` header. The [Protobuf service definition](contracts/proto/flowspace/workspace/v1/workspace_service.proto) defines the routes, and the [resource definition](contracts/proto/flowspace/workspace/v1/workspace.proto) defines their messages.

| Method | Route | Behavior |
| --- | --- | --- |
| `POST` | `/v1/workspaces` | Create a workspace from `{"name":"Example workspace"}` and make the authenticated subject its owner |
| `GET` | `/v1/workspaces/{workspace_id}` | Read a workspace where the authenticated subject has a membership |

Workspace creation also requires an `Idempotency-Key` header, which identifies one create request across retries. Reuse the same key and name when retrying the same creation. A changed name with the same key or an overlapping creation returns HTTP 409. A new intentional creation requires a new key.

For local API experiments, create a user in the `flowspace` realm through Keycloak administration. Set a non-temporary password and complete any required user actions. In your API client, request a token with `POST http://localhost:8080/auth/realms/flowspace/protocol/openid-connect/token` and a form-encoded body:

| Field | Value |
| --- | --- |
| `grant_type` | `password` |
| `client_id` | `workspace-api` |
| `username` | The local user's username |
| `password` | The local user's password |

Use the returned `access_token` in the authorization header for Workspace requests. The local realm enables password-based token requests for API experiments. Browser-session handling remains an open proposal in the architecture document. This repository does not currently contain a Postman collection.

## Development commands

Run `task --list` to see the available repository commands. Integration tests use Testcontainers and require a running Docker runtime. For contribution and quality requirements, read [CONTRIBUTING.md](CONTRIBUTING.md) and [CONSTRAINTS.md](CONSTRAINTS.md).

| Command | Purpose |
| --- | --- |
| `go test ./...` | Run Go tests, including PostgreSQL integration tests |
| `go build ./services/workspace/cmd/...` | Compile the API and migration commands without writing binaries |
| `task fmt` | Format Go source |
| `task check:fast` | Run constraint-checker tests, the quality floor, formatting, and secret scanning |
| `task check:task` | Run local checks, lint, coverage, and reachable dependency vulnerability scanning |
| `task coverage` | Run Go tests and enforce changed-line and total coverage requirements |
| `task vuln` | Scan reachable Go dependencies for known vulnerabilities |
| `task buf -- lint` | Lint Protobuf contracts |
| `task buf -- generate` | Regenerate API messages, clients, and adapters |
| `task sqlc -- generate` | Regenerate PostgreSQL query methods |

Through September 25, 2026, `task check:task` can return success with warnings from failed coverage or dependency vulnerability commands. Those failures block the command beginning September 26, 2026. Read the individual command results before reporting that all checks pass. Do not edit generated files by hand.

## Project documentation

The documents below explain the accepted direction, code placement, and selected tools. Open proposals in the architecture document remain undecided. The project structure describes a target layout, so some directories do not exist yet.

| Document | Contents |
| --- | --- |
| [Architecture](docs/architecture.md) | Product rules, service ownership, consistency, security, environments, and open proposals |
| [Project structure](docs/project-structure.md) | Directory ownership, service layers, and dependency rules |
| [Technology stack](docs/technology-stack.md) | Selected platforms, tools, and Go packages |
| [Architecture decision records](docs/adr/README.md) | Accepted decisions, their rationale, and rejected alternatives |
| [Contributing](CONTRIBUTING.md) | Branches, commits, verification, and pull request requirements |
| [Constraints](CONSTRAINTS.md) | Quality rules and their enforcement commands |

Staging and the environment named production target one Ubuntu host. Those environments do not provide independent host failure domains. Their deployment and recovery procedures remain unimplemented, and the name production does not imply production readiness.
