# Flowspace API

Flowspace is the backend of a work-management app. When it is complete, teams will use workspaces to share projects, tasks, comments, and notifications. The project is also a place to learn how to build and run distributed systems, so it runs on Kubernetes even on a laptop.

## Project status

The project has four planned Go services in one repository:

| Service | What it does | Status |
| --- | --- | --- |
| Identity | Sign-up, email verification, password login, sessions, password recovery, and Google or GitHub login | Available |
| Workspace | Create a workspace and read a workspace that you belong to | Partly available |
| Work | Projects, tasks, assignments, and comments | Planned |
| Notifications | An in-app inbox for workspace activity | Planned |

The [specification index](docs/specs/README.md) lists each module and its status. Local data is disposable. Do not put real user data in the project.

## Prerequisites

The local setup runs on macOS. Install these tools before you start:

| Tool | Use |
| --- | --- |
| [Docker Desktop](https://docs.docker.com/desktop/setup/install/mac-install/) | Runs the local Kubernetes cluster. |
| [k3d](https://k3d.io/stable/), [kubectl](https://kubernetes.io/docs/tasks/tools/install-kubectl-macos/), and [Helm](https://helm.sh/docs/intro/install/) | Create and manage the local cluster. |
| [kubeseal](https://github.com/bitnami-labs/sealed-secrets#kubeseal) and OpenSSL | Create the encrypted local secrets. |
| [Tilt](https://docs.tilt.dev/install.html) | Builds and starts all services. |
| [Go Task](https://taskfile.dev/docs/installation) | Runs the repository commands. |
| [Go 1.27.1](https://go.dev/doc/install) | Builds the services and installs the pinned tools. |
| [Node.js](https://nodejs.org/en/download) with npm | Creates local passwords and checks Markdown. |

The first installation of the tools needs access to the Go and npm package registries.

## Set up the project

Start Docker Desktop. Then run these commands from the repository root:

1. Install the pinned tools into `bin/`:

   ```sh
   task tools:install
   ```

2. Create the local passwords and keys in `.secrets/`:

   ```sh
   task secrets:setup
   ```

3. Create the local cluster. Then select it:

   ```sh
   task cluster:create
   kubectl config use-context k3d-flowspace
   ```

4. Create the encrypted secrets for your cluster:

   ```sh
   task identity:session-tls:setup
   task observability:grafana-admin:setup
   ```

5. Start all services:

   ```sh
   tilt up
   ```

The commands in step 4 change files under `deploy/`, because each cluster has its own encryption key. Send these files for review:

1. Create a short-lived branch.
2. Review the changed files.
3. Commit the changed files.
4. Open a pull request.

Press the space bar in the Tilt terminal to open the Tilt dashboard. The setup is complete when all resources are green.

If you delete the cluster and create it again, the steps are different. Follow the [rebuild procedure for local session TLS](docs/identity-internal-tls.md#rebuild-with-a-new-controller-key). Before you run `tilt up` in that procedure, run `task observability:grafana-admin:setup`.

### Local addresses

When Tilt runs, these addresses are available:

| Address | Service |
| --- | --- |
| `http://localhost:8082` | Identity API |
| `http://localhost:8081` | Workspace API |
| `http://localhost:8080/auth/admin/` | Keycloak admin console, which the Workspace API uses for sign-in until it moves to Identity |
| `http://localhost:8083` | Adminer for the Workspace database. Add `?local=identity` for the Identity database. |
| `http://localhost:3000` | Grafana for logs and traces. Sign in as `admin` with the password in `.secrets/grafana-admin-password`. |

To test the Identity API from start to end, follow the [Bruno smoke test instructions](tests/smoke/bruno/README.md).

### Optional provider login

Google and GitHub login are off by default. If a provider is not set up, its login returns an unavailable error.

To turn on Google login:

1. Create a Google OAuth web client with the redirect URI `http://localhost:8082/v1/provider-login-callbacks/google`.
2. Save the client ID in `.secrets/identity-google-client-id`, without a trailing newline.
3. Save the client secret in `.secrets/identity-google-client-secret`, without a trailing newline.

To turn on GitHub login:

1. Create a GitHub OAuth app with the callback URL `http://localhost:8082/v1/provider-login-callbacks/github`.
2. Save the client ID in `.secrets/identity-github-client-id`, without a trailing newline.
3. Save the client secret in `.secrets/identity-github-client-secret`, without a trailing newline.

## Daily commands

| Command | Purpose |
| --- | --- |
| `task` | List all repository commands. |
| `task cluster:start` | Start the local cluster after you stop it. |
| `task cluster:stop` | Stop the local cluster. |
| `task go:build` | Compile all Go packages. |
| `task go:test` | Run all Go tests. |
| `task check:fast` | Run the fast checks after an edit. |
| `task check:task` | Run all local checks before a pull request. |
| `task markdown:check` | Lint Markdown and find broken local links. |

## Documentation

- [Architecture](docs/architecture.md) describes the product rules and the service boundaries.
- [Project structure](docs/project-structure.md) explains where files go and which packages can depend on each other.
- [Technology stack](docs/technology-stack.md) lists the selected tools and packages.
- [Specifications](docs/specs/README.md) define the behavior of each module.
- [Architecture decisions](docs/adr/README.md) record the accepted decisions and their reasons.
- [Agent workflow](docs/agent-workflow.md) explains how AI agents write and review changes.

## Contributing

1. Read [AGENTS.md](AGENTS.md) and [CONSTRAINTS.md](CONSTRAINTS.md) before you change the project.
2. Make each change on a short-lived branch.
3. Open a pull request, as the [pull request convention](docs/conventions/pull-requests.md) states.

## License

Flowspace is available under the [MIT License](LICENSE).
