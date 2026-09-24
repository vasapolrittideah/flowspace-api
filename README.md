# FlowSpace API

FlowSpace is a work management project for learning how to build and operate distributed systems. This repository contains the Go backend, Protobuf API contracts, and deployment files. The product covers identities, workspaces, projects, tasks, and in-app notifications.

## Install prerequisites

Local development uses macOS. Install [Homebrew](https://brew.sh/) and [Docker Desktop](https://docs.docker.com/desktop/setup/install/mac-install/). Then install the command line tools:

```sh
brew install git go go-task node k3d kubernetes-cli helm tilt
```

Use Go 1.27.1 or newer, as required by `go.mod`. Go, Task, and Node.js run the repository checks. Docker Desktop, k3d, kubectl, Helm, and Tilt support the local Kubernetes environment. Start Docker Desktop before you run integration tests or use the local cluster.

## Set up the repository

Run these commands in a terminal:

```sh
git clone https://github.com/vasapolrittideah/flowspace-api.git
cd flowspace-api
go mod download
task tools:install
task check:fast
```

The first setup needs network access. `task tools:install` installs the pinned project tools in `bin/`. `task check:fast` checks that the tools and basic repository checks run.

For the development workflow, see [CONTRIBUTING.md](CONTRIBUTING.md). For the system design, see [docs/architecture.md](docs/architecture.md).
