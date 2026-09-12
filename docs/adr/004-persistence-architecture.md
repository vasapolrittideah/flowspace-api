# ADR-004: Persistence architecture

## Status

Accepted.

## Date

2026-09-10.

## Context

Membership, ownership, projects, and tasks are relational data with transactional invariants. The project should keep SQL, constraints, queries, and transaction boundaries visible.

## Decision

Use PostgreSQL for primary relational data. Write SQL explicitly, generate typed Go query methods, and keep versioned SQL migrations with the service that owns the data. Application code controls transaction boundaries, and database constraints enforce local invariants.

Each application service runs its own PostgreSQL instance in each environment, with its own database, credentials, migrations, connection pool, storage, and recovery lifecycle. Cross-service access goes through service contracts rather than direct database access or another service's repository implementation.

Driver, query-generation, and migration tools are listed in [technology choices](../technology-choices.md).

## Alternatives Considered

An ORM would hide more of the SQL workflow selected for learning. Manual row mapping would duplicate mechanical work. Ad hoc schema changes would not be reproducible or reviewable. One PostgreSQL instance per environment with separate logical databases would use fewer resources but couple service maintenance, capacity, and recovery.

## Consequences

Schema and query changes are reviewed together, but generation and migration execution remain separate. Deployment must define migration ordering, compatibility, and rollback limits. Cross-service identifiers cannot use database foreign keys; services enforce local integrity and reconcile external references through contracts.

Separate instances improve operational isolation but still share the one physical Ubuntu host, so they do not provide host-level high availability. Resource limits, independent health checks, backups, and restore tests are required per instance.
