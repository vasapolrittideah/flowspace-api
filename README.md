# Flowspace API

Flowspace is a work-management backend for learning how to build and operate distributed systems. The project uses Go services in one repository and one Go module.

## Current status

The Workspace service can create a workspace and read a workspace that the caller belongs to. The local setup uses Keycloak for authentication while the planned Flowspace Identity service is built. Work and Notifications are also planned; their APIs are not available yet. See the [Workspace specification](docs/specs/workspace-creation-and-reading.md) for the available routes and request details.

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

Tilt also starts Adminer at `http://localhost:8083`. Opening that address connects to the Workspace database. Open `http://localhost:8083/?local=identity` to connect to the Identity database. Adminer reads the local database passwords from Kubernetes Secrets. Anyone who can reach the Adminer port can change local data.

After Tilt finishes starting the databases, run `sh scripts/check-adminer-autologin.sh` to check both connections.

Tilt also starts Alloy, Loki, Tempo, and Grafana. The services send their spans to Alloy, and Tempo keeps them for 3 days. Alloy also reads the logs of the Flowspace pods, and Loki keeps them for 7 days. In Grafana, find the log lines of a request with a Loki query such as `{namespace="flowspace-local"} | json | request_id="<request ID>"`. Then open the trace from the `trace_id` link of a line. Tilt forwards Grafana to `http://localhost:3000` through the Kubernetes API, as `kubectl port-forward` does. Grafana has no route outside the cluster.

Sign in as `admin` with the password in `.secrets/grafana-admin-password`. The Grafana admin Secret in Git is sealed with the key of one cluster. After you create a new cluster, run `task observability:grafana-admin:setup` to seal the password again, and commit the changed manifest.

To check that Tempo survives parallel trace searches and answers afterward, run `sh scripts/smoke-tempo-search-local.sh` while Tilt runs. The script sends Identity requests to create traces first.

Google login is optional. To turn it on locally, create a Google OAuth web client with the redirect URI `http://localhost:8082/v1/provider-login-callbacks/google`. Save the client ID in `.secrets/identity-google-client-id` and the client secret in `.secrets/identity-google-client-secret`, without a trailing newline. Tilt then configures Identity for Google login. If either file is missing, Google login returns an unavailable error.

GitHub login is optional in the same way. Create a GitHub OAuth app with the callback URL `http://localhost:8082/v1/provider-login-callbacks/github`. Save the client ID in `.secrets/identity-github-client-id` and the client secret in `.secrets/identity-github-client-secret`, without a trailing newline. If either file is missing, GitHub login returns an unavailable error.

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
- [Agent workflow](docs/agent-workflow.md) explains how AI agents write and review changes.
- [Constraints](CONSTRAINTS.md) define the project quality gates.

## Contributing

Read [AGENTS.md](AGENTS.md) and [CONSTRAINTS.md](CONSTRAINTS.md) before making changes. Work on a short-lived branch, run the relevant checks, and submit a pull request using the [PR convention](docs/conventions/pull-requests.md).

## License

Flowspace is available under the [MIT License](LICENSE).
