# FlowSpace

An all-in-one work management platform — projects, tasks, collaboration — built
as Go microservices in a monorepo. The purpose of this repository is to learn
how real enterprise systems work by building one, so the enterprise-grade
choice is the deliverable, not the shortest path to a working feature.

## Constraints that shape every answer

- One Ubuntu node runs both staging and production: 32 GB RAM, 500 GB SSD, one
  GPU. There is no managed cloud and no budget for one.
- One maintainer. Any process that needs a second person is theatre.
- Development is on macOS (arm64); the server is amd64.

Do not offer a simpler alternative to a pattern that was chosen deliberately —
the outbox, sagas, relationship-based authorization, dynamic secrets. The
complexity is the point. Do push back on anything whose cost lands on the
32 GB budget without earning it.

## Decisions live in docs/adr

Every ADR under `docs/adr/` is binding. Read the relevant one before changing
behaviour it governs.

- A change that contradicts an accepted ADR is a stop-and-say-so, never a
  judgement call.
- A change that needs a new decision carries its ADR in the same pull request.

### Writing an ADR

Copy `docs/adr/template.md`. Number sequentially and never reuse a number. The
filename slug carries no articles: `0009-one-go-module-for-repository.md`.

- Cite another ADR by number only if that file already exists. Name an
  undecided thing by its subject — "the migration tool" — never by a number
  reserved in advance.
- References point backwards. Never edit an older ADR to point forward at a
  newer one.
- Context, Decision and Consequences are immutable once Accepted. Status and
  cross-reference links are maintained in place.
- Section order follows the template exactly.
- Say what the decision costs. An ADR with nothing under "Negative" is
  unfinished, not clean.

## Repository layout

```text
api/openapi.yaml     public HTTP contract, source of truth
proto/               event and RPC contracts
gen/                 generated code, committed to the repository
services/<name>/
  cmd/               the only code outside internal/
  internal/          domain, app, adapter
pkg/                 infrastructure only, never domain types
deploy/              kustomize base and per-environment overlays
docs/adr/
```

One `go.mod` at the root. No `go.work`, no per-service modules, no `replace`.

### Boundaries

- A service's code lives under its own `internal/`. That is the boundary, and
  the compiler enforces it.
- Services never import one another. They communicate through the contracts in
  `proto/`.
- `pkg/` holds telemetry setup, interceptors, the outbox helper, error types. A
  domain type appearing in `pkg/` means a boundary is in the wrong place.

## Commits and pull requests

Branch commits are free-form and are discarded at merge. Write what state the
code is in, on one line:

```text
status transitions work, tests still red
revert to enum, lifecycle states got messy
```

Say whether the tests pass only when that is actually known from this session.
Do not run the suite to find out and do not guess — omit the clause instead. A
wrong claim about test state is worse than none, because it is what gets
trusted when scanning back for a point to return to.

Never stage anything resembling a credential, a `.env` file, a build artifact,
a vendored directory, or a file over 1 MB. Name it and ask. A secret that
reaches a pushed branch is on the remote whether or not the squash carries it
to main.

Do not tidy branch history, and do not put reasoning in a commit body — that
belongs in the pull request description, which survives the squash.

The pull request title is a Conventional Commit and becomes the commit on main.
The scope is the service directory, or `repo` for repository-wide changes, and
`!` marks a breaking contract change:

```text
feat(workitem)!: replace status enum with lifecycle states
```

A title is a headline, not a sentence. Drop an article when removing it changes
only the length, and keep it when the phrase becomes ambiguous or means
something else without it — `revert to enum` and `revert to the enum` are not
the same claim. This governs pull request titles, branch commit subjects and
ADR filename slugs, which never carry one. It does not govern prose: an ADR
heading is read as language, so `0009-one-go-module-for-repository.md` is
titled "One Go module for the whole repository".

## Markdown

`markdownlint-cli2 "**/*.md"` must pass. Prose wraps at 80 columns. Tables are
exempt from the width limit and are padded so the pipes line up.
