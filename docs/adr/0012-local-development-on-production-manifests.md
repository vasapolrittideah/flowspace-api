# ADR-0012: Develop locally on the same manifests as production

- **Status:** Accepted
- **Date:** 2026-09-05
- **Deciders:** Vasapol Rittideah
- **Tags:** `delivery`

## Context

Development happens on a macOS laptop; staging and production are one Ubuntu
node running k3s. Every difference between those two descriptions of the system
is a place a defect can hide until deployment.

The common arrangement — Docker Compose locally, Kubernetes in production —
creates exactly the problem this project already refused once. ADR-0010
rejected having two descriptions of one HTTP API with nothing linking them.
Compose beside Kustomize is the same shape: two files describing one system,
diverging quietly, with the only detector being a failed deployment.

Some differences are unavoidable. There is no Vault on a laptop, no Cloudflare
in front of it, no GPU on an Apple machine, and not enough memory to run
everything at once. The question is not how to eliminate the differences. It is
how to make the list of them short, explicit, and unable to grow by accident.

The other constraint is the loop. If a code change takes a minute to appear,
the local environment stops being used, and no amount of parity matters after
that.

## Decision

Local development runs **k3d** — the same k3s that runs on the server, in
Docker — against the **same Kustomize base** as staging and production, through
a `local` overlay. **Tilt** drives the loop.

### The overlay is the whole difference

| Concern     | Local              | Staging and production      |
| ----------- | ------------------ | --------------------------- |
| Replicas    | One                | Two or more                 |
| Secrets     | Kubernetes Secret  | Vault injection             |
| Secret path | `/vault/secrets/*` | `/vault/secrets/*`          |
| Edge        | Tilt port forward  | Cloudflare and the gateway  |
| Images      | Built by Tilt      | Pinned by digest (ADR-0011) |
| Data        | Seed job           | Real data                   |
| Inference   | Stubbed            | Local model on the GPU      |

That table is exhaustive. Anything else that differs between local and
deployed is a defect in the overlay, not a local convenience, and a change to
domain configuration made "so it works locally" is a finding rather than a fix.

The secret path is the row that earns its place. Vault is absent locally and a
plain Kubernetes Secret takes its place, but it is mounted at the path Vault
would have used, so the application reads a file from one location in every
environment and contains no branch on where it is running.

### Infrastructure runs in the cluster

PostgreSQL, the broker, OpenFGA and the cache run from the same manifests as
the deployed environments, inside the cluster. Not installed on the host. This
is what makes connection strings, service DNS names, readiness gating and
startup ordering real locally instead of theoretical, and those are the things
that break on first deployment.

Migrations run as the same job with the same tool, so an ordering mistake
surfaces on the laptop.

### Tilt compiles on the host

Go builds on the host, using its own cache, and Tilt syncs the binary into a
running container and restarts the process. There is no image build in the
loop, which is the difference between a few seconds and a minute.

The binary is built for the container's architecture — `arm64` on Apple
silicon, taken from the host rather than hard-coded. CI builds `linux/amd64`
only. No multi-architecture manifests are produced, because nothing consumes
them.

Services not currently being worked on do not start automatically and are
brought up from the Tilt interface. On a machine that cannot hold the whole
system, selective startup is part of the design rather than a setting.

### What parity does not cover

Vault's dynamic credentials, the Cloudflare edge, GPU inference, realistic data
volumes, and any behaviour that needs more than one replica — leader election
among them — are not reproduced locally and are not pretended to be. They are
exercised in the preview environment that every pull request gets. Local is
fast and incomplete; the preview environment is complete and slower. Confusing
the two is how a gap becomes a surprise.

## Consequences

### Positive

- One set of manifests for every environment, so a manifest defect is found on
  a laptop rather than in a pipeline.
- Onboarding is `tilt up`, with no second system to describe or maintain.
- Application code has one path for configuration and secrets, because the
  environment differences were absorbed by the overlay rather than by an `if`.
- The distribution running the manifests locally is the one that runs them on
  the server.

### Negative / accepted costs

- **The production image is not built in the inner loop.** Tilt syncs a binary
  into a development image, so a change that breaks the production Dockerfile
  is caught by CI minutes later rather than locally in seconds.
- Two Dockerfiles can drift. The development one is kept trivial — a base and a
  binary — so that there is little in it to drift.
- Architecture differs. A defect that only appears on `amd64` is invisible
  locally, which is rare in Go and not impossible.
- k3d with the full stack is heavy on a laptop, which is why selective startup
  is mandatory rather than a convenience.
- The Tiltfile is code, and it needs maintaining like code.

### Neutral

- Because local uses the same base, a change to the base is tested by everyone
  running `tilt up` before it ever reaches a cluster.

## Alternatives considered

| Option                                             | Why not                                                                                                                                                                                    |
| -------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| Compose locally, Kubernetes in production          | Two descriptions of one system with nothing connecting them, which is the same failure this project already refused in ADR-0010; every manifest defect is then found in CI at the earliest |
| Services on the host, infrastructure in containers | The fastest loop available, and it exercises none of the configuration loading, service discovery or startup ordering that actually breaks in deployment                                   |
| A shared remote development cluster                | Requires the network to work, serialises everyone on one environment, and turns a compile error into a deployment                                                                          |
| Preview environments only, no local cluster        | Correct but slow. A minute per iteration is not an inner loop, and an inner loop that hurts stops being used                                                                               |
| kind or minikube instead of k3d                    | Equivalent in capability. k3d is chosen because the target runs k3s, so the distribution under the manifests is the same one that will run them                                            |

## Revisit when

- The laptop can no longer hold a useful subset of the system, at which point
  the inner loop moves to a remote development cluster and this decision is
  replaced rather than amended.
- The development and production Dockerfiles drift far enough to cause a real
  incident, which would mean the development image needs to be the production
  image with a different entrypoint.
- The overlay grows a difference that is not in the table above, which means
  either the table is wrong or the deployment is.

## References

- [Tilt documentation](https://docs.tilt.dev/)
- [k3d](https://k3d.io/)
- [Kustomize overlays](https://kubectl.docs.kubernetes.io/references/kustomize/glossary/#overlay)
