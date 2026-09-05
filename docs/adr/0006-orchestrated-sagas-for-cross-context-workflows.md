# ADR-0006: Run cross-context workflows as orchestrated sagas

- **Status:** Accepted
- **Date:** 2026-09-05
- **Deciders:** Vasapol Rittideah
- **Tags:** `domain`

## Context

A transaction covers one aggregate in one database. Several FlowSpace
operations do not fit inside one: creating a project from a template reserves a
key, creates the project, creates several dozen work items, grants memberships
and notifies people — across three bounded contexts. Deleting a workspace and
moving a project between workspaces have the same shape.

Doing this as a sequence of calls from the BFF fails in a specific and
expensive way. When step four fails, steps one through three are already
committed and durable. The caller receives an error describing nothing useful,
the system holds a project that half exists, and there is no record anywhere of
what was being attempted, so nothing can finish it or undo it. The partial
state is indistinguishable from a legitimate one.

What is needed is not a bigger transaction. It is a durable record of an
*intention*, which survives restarts, drives itself forward, and knows how to
walk backwards when it cannot.

## Decision

Cross-context workflows are **orchestrated sagas**: an explicit state machine,
owned by the service that owns the business outcome, persisted in that
service's database, and driven by the outbox and inbox from ADR-0005.

No new infrastructure is introduced. A saga step is a command written to the
outbox in the same transaction as the saga's state change; a step result is an
event consumed through the inbox. The saga is therefore exactly as durable as
everything else, and needs no separate delivery mechanism.

### The workflow is one readable artifact

```text
CreateProjectFromTemplate
  1  reserve_project_key      compensate: release_project_key
  2  create_project           compensate: archive_project
  3  create_work_items        compensate: delete_work_items
  4  grant_memberships        compensate: revoke_memberships
  ─  pivot ───────────────────────────────────────────────────
  5  notify_members           roll forward only
```

A step may not be added without its compensation. Compensations run in reverse
order, and each one is a **new business fact, not an undo**: the project becomes
archived, it does not become never-created. Anyone who saw it saw something
real.

Steps before the pivot are compensatable. The pivot is the point after which
the world has been changed in a way that cannot be taken back — a message sent,
an external system called. Past it the saga can only roll forward, retrying
until it succeeds or a human intervenes. Every saga states where its pivot is,
and a saga whose irreversible step is not last is a design error.

### Persistence and control

```sql
create table saga_instance (
  workspace_id uuid not null,
  id           uuid not null,          -- UUIDv7
  type         text not null,
  step         smallint not null,
  status       text not null,          -- running|compensating|done|failed
  payload      jsonb not null,
  deadline_at  timestamptz not null,
  version      integer not null default 1,
  updated_at   timestamptz not null default now(),
  primary key (workspace_id, id)
);
```

The table follows ADR-0002 in full, including row level security. The sweeper
that finds overdue sagas needs to look across tenants, and does so through a
role-scoped policy rather than an exemption from the model:

```sql
create policy saga_tenant on saga_instance
  using (workspace_id = current_setting('app.workspace_id')::uuid);

create policy saga_sweeper on saga_instance for select
  to flowspace_saga_sweeper using (true);
```

The sweeper's grant covers only `workspace_id`, `id`, `status` and
`deadline_at`. It can see that a saga is late; it cannot see what the saga is
about.

Three rules make the machine safe to restart:

- **Every step is idempotent**, keyed by saga identifier and step number. A
  redelivered step result advances the saga once.
- **Every step has a deadline.** Expiry before the pivot starts compensation;
  expiry after it raises an alert and keeps retrying. A saga with no deadline
  does not fail, it disappears.
- **Workflow state is only in the table.** Never in a goroutine, never in
  memory. A restart mid-workflow resumes from the row.

### Isolation does not exist, so it is replaced by convention

A saga is atomic, consistent and durable, but **not isolated**. Other actors
observe intermediate states. The countermeasure is a semantic lock: an
aggregate created by a saga carries a status of its own, and operations from
elsewhere refuse to act on a row that is still `pending`. This is discipline
enforced by domain code, and a missing check is a real defect, not a
theoretical one.

### What is not a saga

A workflow is a saga only if it crosses contexts *and* needs compensation. "Do
A, then let B react" is an event, and modelling it as a saga adds a state
machine that never branches. Sagas live in the service that owns the outcome;
there is no central saga service, because that is a distributed monolith with
extra latency.

## Consequences

### Positive

- The workflow is one file that can be read, reviewed, and unit-tested as a
  state machine with no infrastructure at all.
- An interrupted workflow resumes; nothing depends on a process staying alive.
- `select * from saga_instance where status = 'running'` answers "what is stuck
  right now", which is normally the hardest question in an async system.
- Zero new moving parts: the transport is the outbox that already exists.

### Negative / accepted costs

- The orchestrator knows the command surface of other contexts. That coupling
  is real; it is accepted because it is explicit and lives in one place, rather
  than being spread invisibly across every participant.
- **Compensation code runs rarely and therefore rots.** The only defence is
  that CI exercises every compensation path with injected step failures. A
  compensation that has never executed is not known to work.
- Callers get an acknowledgement, not a result. The API returns "accepted" with
  a saga identifier, and the interface has to show pending state honestly
  instead of pretending the work is done.
- Semantic locks are convention. The compiler cannot enforce them.
- Each saga is code, not configuration, so a new workflow is a deployment.

### Neutral

- `saga_oldest_running_seconds` and a counter of executed compensations are the
  two observables. A rise in the second is an upstream failure surfacing here
  before it surfaces to users.

## Alternatives considered

| Option                                                   | Why not                                                                                                                                                                                                     |
| -------------------------------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Choreography — each service reacts to the previous event | The workflow exists in no single artifact. Adding a step means editing several services and hoping; understanding a failure means reconstructing the sequence from logs after the fact                      |
| Two-phase commit across services                         | Rejected for the reasons in ADR-0005, and additionally it would hold locks across service boundaries for the duration of a workflow measured in seconds                                                     |
| Temporal or Cadence                                      | Better ergonomics than this decision on every axis except cost: it adds a stateful cluster with its own datastore to a 32 GB node, and a second durable-execution model alongside the outbox already in use |
| Camunda or another BPMN engine                           | A JVM cluster, and the workflow moves into a diagram that the compiler cannot check and the test suite cannot exercise                                                                                      |
| A synchronous call chain in the BFF                      | Partial failure leaves committed state with no record of the intent behind it; the caller's HTTP timeout becomes the system's consistency model                                                             |

## Revisit when

- More than roughly ten saga types exist, or one needs durable timers spanning
  days, human approval steps, or versioned long-running instances. Those are
  the problems a workflow engine solves and this design does not.
- Compensation logic begins to be copied between sagas, which means the
  workflow layer has grown a domain of its own.
- A saga routinely outlives the broker retention it depends on to resume.

## References

- Garcia-Molina and Salem, *Sagas* (1987)
- [Saga pattern](https://microservices.io/patterns/data/saga.html)
- [PostgreSQL CREATE POLICY](https://www.postgresql.org/docs/current/sql-createpolicy.html)
