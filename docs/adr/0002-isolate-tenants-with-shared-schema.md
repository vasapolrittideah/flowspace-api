# ADR-0002: Isolate tenants with a shared schema keyed by workspace

- **Status:** Accepted
- **Date:** 2026-09-05
- **Deciders:** Vasapol Rittideah
- **Tags:** `data`, `security`

## Context

FlowSpace is multi-tenant. An organisation signs up, creates a *workspace*, and
everything it owns — projects, work items, comments, automations — lives inside
that workspace. A user account is global and may belong to several workspaces.

Cross-tenant data leakage is the most expensive bug class a SaaS product can
ship: it is silent, it is found by customers rather than by tests, and it
cannot be un-shipped. An isolation model must therefore be judged by what
happens when a developer forgets something, not by what happens when they get
it right.

The deployment target is a single Ubuntu node running one PostgreSQL cluster.
Physical isolation per tenant is unavailable at any price this project is
willing to pay, so the only open question is how isolation is expressed inside
one database.

This cannot be deferred. `workspace_id` participates in primary keys, foreign
keys and every index; adding it later rewrites every migration and every query
in the system.

## Decision

The **workspace is the tenant boundary**. All tenant-owned data lives in one
shared database and one shared schema, discriminated by `workspace_id`.

Three rules, ordered by who enforces them:

1. **The schema enforces referential integrity.** Every tenant-scoped table
   carries `workspace_id uuid not null`. It is the leading column of the
   primary key, and every foreign key between tenant-scoped tables is composite
   and carries `workspace_id` through. A cross-tenant reference becomes
   impossible to store, not merely unlikely.

2. **The application scopes explicitly.** Repository methods take `workspaceID`
   as a required argument. It is never read from an ambient context value or a
   package-level variable. A query that omits it does not compile.

3. **Row-Level Security is the backstop.** Every tenant-scoped table has RLS
   enabled with a policy on `current_setting('app.workspace_id')`, which the
   application sets with `SET LOCAL` at the start of every transaction.

```sql
create table work_items (
  workspace_id uuid not null references workspaces (id),
  id           uuid not null,                 -- time-ordered UUID
  project_id   uuid not null,
  title        text not null,
  version      integer not null default 1,    -- optimistic locking
  primary key (workspace_id, id),
  foreign key (workspace_id, project_id)
    references projects (workspace_id, id)    -- composite: cannot cross tenants
);

alter table work_items enable row level security;
create policy tenant_isolation on work_items
  using (workspace_id = current_setting('app.workspace_id')::uuid);
```

Naming: the column is `workspace_id`, not `tenant_id`. "Workspace" is the term
the domain, the UI and the public API already use, and the ubiquitous language
does not get a second vocabulary for the database.

Global tables, owned by the identity service and exempt from all three rules:
`users`, `credentials`, `sessions`, and `workspaces` itself. The bridge table
`workspace_members` is tenant-scoped and keyed `(workspace_id, user_id)`.

The following are part of this decision, not implementation detail:

- The application connects as a role that **owns no tables** and holds neither
  `SUPERUSER` nor `BYPASSRLS`. Table ownership belongs to the role that runs
  schema migrations. RLS never applies to a table's owner, so reusing one role
  for both silently disables the entire backstop.
- Pooling runs PgBouncer in transaction mode (CloudNativePG `Pooler`).
  `SET LOCAL` is transaction-scoped and safe under transaction pooling; plain
  `SET` is not, and is forbidden.
- A transaction that forgets `SET LOCAL` returns zero rows rather than another
  tenant's rows. The failure mode is fail-closed and loud in tests.

## Consequences

### Positive

- One cluster, one migration run, one connection pool.
- The database rejects a cross-tenant foreign key outright.
- A forgotten `where workspace_id = $1` returns nothing instead of leaking.
- Index entries cluster by workspace, so a large tenant does not scatter a
  small tenant's rows across every index page.

### Negative / accepted costs

- **No per-tenant restore.** Recovering one workspace means extracting rows
  from a cluster-wide backup, not restoring a backup. The procedure belongs to
  the backup and restore ADR, which must exist before the first real user
  does.
- **Noisy neighbours are real.** One workspace with 500k work items degrades
  planning for everyone. The answer is partitioning later, not isolation now.
- Every index is 16 bytes per row wider, and composite foreign keys make
  migrations noticeably more verbose to write and to read.
- `SET LOCAL` is boilerplate on every transaction. It is centralised in one
  `pkg/` helper, and bypassing that helper is a review-blocking defect.

### Neutral

- Extracting one tenant to a dedicated cluster later stays possible: it is a
  dump filtered by `workspace_id`, not a schema change.

## Alternatives considered

| Option                                    | Why not                                                                                                       |
| ----------------------------------------- | ------------------------------------------------------------------------------------------------------------- |
| Database per tenant                       | Connection count and migration fan-out scale with tenant count; unusable past a few dozen tenants on one node |
| Schema per tenant                         | Same fan-out, plus `pg_catalog` bloat and no cross-tenant reporting query                                     |
| Shared schema, application filtering only | One forgotten predicate is a silent cross-tenant leak with no second line of defence                          |
| Shared schema with scalar primary keys    | Nothing stops a work item in workspace A referencing a project in workspace B                                 |

## Revisit when

- A single workspace exceeds roughly 20% of total database size or IOPS.
  Partition the largest tables by `workspace_id` before considering extraction.
- A customer requires data residency, a dedicated encryption key, or a
  contractual guarantee of physical isolation. That is a business event; it
  supersedes this ADR rather than amending it.

## References

- [PostgreSQL Row Security Policies](https://www.postgresql.org/docs/current/ddl-rowsecurity.html)
- [PgBouncer pooling modes](https://www.pgbouncer.org/features.html)
- [Azure Architecture Center — multitenant data models](https://learn.microsoft.com/en-us/azure/architecture/guide/multitenant/approaches/storage-data)
