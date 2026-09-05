# ADR-0008: Model authorization as relationships in OpenFGA

- **Status:** Accepted
- **Date:** 2026-09-05
- **Deciders:** Vasapol Rittideah
- **Tags:** `security`, `domain`

## Context

Authorization in a project management product is not a set of roles. The
ordinary cases already break the role model:

- Someone administers a workspace but is only a viewer on one project inside
  it, because that project is private.
- Someone is assigned a single work item in a project they otherwise cannot
  open, and must be able to see that one item.
- A guest is invited to exactly one project and nothing else in the workspace.
- Permissions inherit down workspace, project and work item, with exceptions at
  every level.

Expressing this with roles and a permission table means a bespoke subquery per
resource type, and it fails completely on the second question. There are two
questions, not one: **may this user do this to this object**, and **which
objects may this user do this to**. The first can be brute-forced. The second
cannot — a board with ten thousand work items cannot be answered one check at a
time, and it is the question every list endpoint asks.

Tenant isolation does not help here. ADR-0002 keeps workspace A's rows away
from workspace B, but everyone inside a workspace passes that check and most of
them still cannot see most of it. Isolation and authorization are different
problems, often confused because both end in a `WHERE` clause.

ADR-0007 raises the stakes: a subscription is authorised once at connect time
and again at every fetch, so whatever answers these questions sits on the hot
path of everything.

## Decision

Authorization is **relationship-based**, in the style of Google's Zanzibar,
served by **OpenFGA** running as its own service against its own database.

The model is a versioned file in the repository, deployed like a schema
migration:

```text
type workspace
  relations
    define owner: [user]
    define member: [user] or owner

type project
  relations
    define workspace: [workspace]
    define lead: [user]
    define contributor: [user] or lead
    define viewer: [user] or contributor or member from workspace
    define can_view: viewer
    define can_edit: contributor

type work_item
  relations
    define project: [project]
    define assignee: [user]
    define can_view: can_view from project
    define can_edit: can_edit from project or assignee
```

No domain code contains a role comparison. There is one `can(...)` helper and
one middleware, and a permission check written anywhere else is a defect.

### The window always errs toward denial

OpenFGA is a second store, so writing a tuple and committing a row is another
dual write. It is deliberately **not** solved with the outbox from ADR-0005,
because the two failure modes are not symmetric. A tuple pointing at an object
that does not exist is harmless: the fetch that follows finds no row. A missing
tuple denies a user access to something they own.

The ordering rule follows from that asymmetry, and it is the opposite of what
the outbox does, on purpose:

- **Granting** — creating an object, adding a member, raising a role: write the
  tuple first, then commit. A failed commit leaves an orphan tuple that grants
  access to nothing.
- **Revoking** — deleting an object, removing a member, lowering a role: delete
  the tuple first, then commit. A failed commit leaves a user briefly unable to
  reach something they still own.

In both directions the transient state is the more restrictive one. A
reconciler sweeps orphan tuples on a schedule, and nothing depends on it
running promptly.

### The tuple store is derived, not authoritative

PostgreSQL remains the source of truth for who belongs to what. The tuple store
is an index over that, and **must be fully rebuildable from the domain
database**. A modelling choice that cannot be reconstructed from PostgreSQL is
rejected for that reason alone. This is what makes drift recoverable rather
than a permanent mystery, and what makes a restore of the domain database a
complete restore.

### Checking and listing are different operations

- **Point checks** are batched per request and cached briefly. A request that
  immediately follows a tuple write asks for the stronger consistency option
  rather than the low-latency default.
- **Listing is coarse in the database and fine on the page.** The database
  filters by tenant and by the containers the user has any relationship to; a
  batched check then runs over the page that comes back. Relationship listing
  is used for containers a user can see — workspaces, projects — never for
  leaf objects such as work items, whose sets are unbounded.

That split is a permanent constraint on every list endpoint, not an
optimisation to apply later. An endpoint that cannot express a coarse filter is
a modelling problem, not a performance problem.

## Consequences

### Positive

- The entire permission model is one reviewable file. Guests, link sharing and
  private projects inside administered workspaces become model changes rather
  than schema changes.
- One mechanism answers both the check and the list question.
- Adding a resource type does not add a bespoke authorization query.
- Restoring PostgreSQL restores authorization, because tuples are derived.

### Negative / accepted costs

- **A network call on the hot path of nearly every request.** Batching and a
  short cache make it affordable; they do not make it free.
- **A second store that can drift.** This is a class of bug that did not exist
  before. Rebuildability and a reconciler are the response to it, not a
  guarantee against it.
- Another stateful service on a node already accounted for to the gigabyte.
- "Why can I not see this" is debugged by reading tuples rather than SQL, which
  needs tooling and a habit nobody has yet.
- Model versions are immutable in OpenFGA, so a model change is a deployment
  step with an ordering relative to the code that depends on it.

### Neutral

- The ordering rule above is the inverse of the outbox rule in ADR-0005. Both
  follow the same principle — make the transient state the safe one — and it
  points in opposite directions because the risks are not symmetric.

## Alternatives considered

| Option                         | Why not                                                                                                                                                                                            |
| ------------------------------ | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Roles on a membership table    | Cannot express a viewer of one project inside a workspace the same person administers, and every per-object exception becomes a row anyway — arriving at access control lists without admitting it |
| Access control rows per object | Every list endpoint becomes a join against a growing grant table, and each new resource type needs new schema plus a new bespoke query                                                             |
| Casbin evaluated in process    | No network hop, but the model answers one object at a time; there is no answer to which objects a subject may act on without enumerating them                                                      |
| SpiceDB                        | An equally valid Zanzibar implementation with a stronger consistency story. Chosen against on footprint and on the readability of the modelling language, not on capability                        |
| Open Policy Agent              | Strong at policy over request attributes, but it is not a relationship store; nested inheritance would become data that something else has to compute and feed it                                  |

## Revisit when

- Check latency at p99 becomes a meaningful share of the request budget. The
  next step is a local decision cache with explicit invalidation, before it is
  a different product.
- Drift is observed despite reconciliation, which would mean the ordering rule
  is being bypassed somewhere and tuple writes belong in the outbox after all.
- The model stops fitting in one file a person can read in one sitting.

## References

- [Zanzibar: Google's Consistent, Global Authorization System](https://research.google/pubs/pub48190/)
- [OpenFGA documentation](https://openfga.dev/docs)
- [OpenFGA modeling guide](https://openfga.dev/docs/modeling/getting-started)
