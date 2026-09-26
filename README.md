# FlowSpace API

FlowSpace is a work-management backend for learning how to build and operate distributed systems. The project uses Go services in one repository and one Go module.

## Current status

The Workspace service can create a workspace and read a workspace that the caller belongs to. The local setup uses Keycloak for authentication while the planned FlowSpace Identity service is built. Work and Notifications are also planned; their APIs are not available yet. See the [Workspace specification](docs/specs/workspace-creation-and-reading.md) for the available routes and request details.

Local data is disposable, and the project is not ready for real user data.

## Prerequisites

The local setup runs on macOS. Install these tools before you start:

- [Docker Desktop](https://docs.docker.com/desktop/setup/install/mac-install/), [k3d](https://k3d.io/stable/), [kubectl](https://kubernetes.io/docs/tasks/tools/install-kubectl-macos/), [Helm](https://helm.sh/docs/intro/install/), and [Tilt](https://docs.tilt.dev/install.html) for the local Kubernetes environment.
- [Go Task](https://taskfile.dev/docs/installation) for the repository commands.
- [Node.js](https://nodejs.org/en/download) with [npm](https://docs.npmjs.com/downloading-and-installing-node-js-and-npm/) for local password setup and Markdown checks.
- [Go 1.27.1](https://go.dev/doc/install) for Go development and installation of the pinned repository tools.

The first installation of the pinned tools and Markdown linter needs access to the Go and npm package registries.

## Quick start

For the first setup, start Docker Desktop, then run these commands from the repository root:

```sh
task tools:install
task secrets:setup
task cluster:create
tilt up
```

Tilt starts Keycloak, PostgreSQL, migrations, and the Workspace API. It forwards the Workspace API to `http://localhost:8081` and Keycloak to `http://localhost:8080`.

## Development

| Command | Purpose |
| --- | --- |
| `task` | List the available repository commands. |
| `task tools:install` | Install pinned Go tools into `bin/`. |
| `task go:build` | Compile all Go packages. |
| `task go:test` | Run all Go tests. |
| `task check:fast` | Run checks after an edit. |
| `task check:task` | Run the local handoff checks. |
| `task markdown:check` | Lint Markdown and check local links. |

## Documentation

- [Architecture](docs/architecture.md) describes the accepted system direction and open proposals.
- [Project structure](docs/project-structure.md) explains file ownership and dependency boundaries.
- [Technology stack](docs/technology-stack.md) lists selected tools and packages.
- [Specifications](docs/specs/README.md) define module behavior.
- [Architecture decisions](docs/adr/README.md) record accepted decisions.
- [Constraints](CONSTRAINTS.md) define the project quality gates.

## Contributing

Read [AGENTS.md](AGENTS.md) and [CONSTRAINTS.md](CONSTRAINTS.md) before making changes. Work on a short-lived branch, run the relevant checks, and submit a pull request using the [PR convention](docs/conventions/pull-requests.md).

## License

FlowSpace is available under the [MIT License](LICENSE).
