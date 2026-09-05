# ADR-0009: One Go module for the whole repository

- **Status:** Accepted
- **Date:** 2026-09-05
- **Deciders:** Vasapol Rittideah
- **Tags:** `delivery`

## Context

FlowSpace is several services that deploy independently but are always built
from one commit, by one person, sharing generated contract code and a small
amount of infrastructure code.

The usual instinct in a Go monorepo is a module per service, tied together with
a workspace file. That buys the ability to hold different dependency versions
per service, and charges a `go.mod` and `go.sum` per service, `replace`
directives pointing at shared code, and a bump-and-propagate step every time
that shared code changes. For one maintainer this is a coordination protocol
with nobody on the other end.

The real question is not whether the ceremony is worth it. It is what is lost
without it, because the thing a module boundary provides — a wall between
services that the compiler enforces — is genuinely worth having and would be
expensive to give up.

## Decision

There is **one `go.mod` at the repository root**. No `go.work`, no per-service
modules, no `replace` directives.

Boundaries between services are enforced by three mechanisms, in descending
order of strength.

### The compiler still enforces the wall

Go's `internal/` rule is path-based, not module-based. A package under
`services/identity/internal/` is importable only by packages rooted at
`services/identity/`, in a single module exactly as in separate ones.

```text
services/
  identity/
    cmd/main.go            the only file outside internal/
    internal/
      domain/              importable only from services/identity/...
      app/
      adapter/
  workitem/
    cmd/main.go
    internal/...
pkg/                       infrastructure only, never domain types
gen/                       committed buf and oapi-codegen output
```

Everything belonging to a service lives under its `internal/`. This is the
whole boundary, and it costs nothing.

### Lint covers what the layout does not

A `depguard` rule denies any import from one `services/…` tree into another,
which catches the case that defeats the compiler rule: someone creating a
package outside `internal/`. It is a net under the layout, not the layout
itself.

### Shared code has a definition, not a location

`pkg/` holds infrastructure concerns only — telemetry setup, RPC interceptors,
the outbox helper, error types. **No domain types.** A type that two services
both need is either part of the contract between them, in which case it belongs
in `proto/`, or evidence that the boundary is in the wrong place. A `pkg/model`
package is how a monorepo becomes a distributed monolith, and its absence is
part of this decision.

### Everything shares one dependency version

There is one version of every library across every service. An upgrade is one
commit that moves everything, and a vulnerability is fixed once. The cost is
that a risky upgrade cannot be staged one service at a time.

### Generated code is committed

`gen/` is in version control rather than produced during the build. A fresh
clone compiles without `buf` installed, and a change to a contract shows up as
a reviewable diff in the pull request that makes it — which is how a breaking
change becomes visible to a human. CI regenerates and fails if the result
differs from what was committed, so the tree cannot go stale.

## Consequences

### Positive

- `go build ./...` and `go test ./...` work from a clean clone with no
  workspace configuration and no editor setup.
- A change that crosses a service boundary — a contract and both sides of it —
  is one atomic commit that CI evaluates as a unit.
- No version skew between a service and the shared code it uses, because there
  is only one version of anything.
- The boundary that matters is enforced by the compiler, for free.

### Negative / accepted costs

- A dependency cannot be pinned to an old version for one service while another
  moves ahead. This is the one thing separate modules do that this cannot.
- A change to `go.mod`, `pkg/` or `gen/` invalidates every service, so
  change-detection in CI needs a shared catch-all covering exactly those paths.
  Omitting one produces images that silently do not contain the change.
- Nothing in the repository can be imported by anything outside it without
  first being split out into its own module.
- Generated code in version control produces merge conflicts in files nobody
  edits.

### Neutral

- Splitting a service out later is mechanical: move the directory, add a
  `go.mod`, and the `internal/` boundary it already respects becomes a module
  boundary unchanged.

## Alternatives considered

| Option                                                | Why not                                                                                                                                                                          |
| ----------------------------------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| A module per service with `go.work`                   | Buys per-service dependency versions, and charges a `go.mod` and `go.sum` per service, `replace` directives for shared code, and a version bump ritual every time `pkg/` changes |
| A module per service, shared code released separately | Every change to shared code becomes a release followed by upgrades in each consumer, which is ceremony without a second person to protect from it                                |
| A repository per service                              | A change that crosses a contract becomes several pull requests that cannot land atomically, which is the coordination cost of a team, paid by one person                         |
| Bazel or another build graph                          | Solves incremental builds for a repository large enough to feel the problem; here it adds a build system to learn and maintain for something that compiles in seconds            |

## Revisit when

- Two services genuinely require different major versions of the same
  dependency at the same time. That is the one problem this decision cannot
  express, and it is the signal to split.
- A full-repository build or test run becomes slow enough to change behaviour —
  when someone starts skipping it locally, it is already too slow.
- Any part of this repository needs to be consumed by something outside it.

## References

- [Go Modules Reference](https://go.dev/ref/mod)
- [Internal Directories](https://pkg.go.dev/cmd/go#hdr-Internal_Directories)
- [Go workspaces](https://go.dev/blog/get-familiar-with-workspaces)
- [golangci-lint depguard](https://golangci-lint.run/usage/linters/#depguard)
