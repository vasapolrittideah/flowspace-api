# ADR-0014: Relational data uses explicit SQL

## Status

Accepted

## Date

2026-09-14

## Context

Workspace membership, ownership, projects, tasks, and assignments are relational data with transactional invariants. The project is intended to make SQL, database constraints, queries, and transaction boundaries visible while avoiding repetitive mechanical row mapping.

## Decision

Use PostgreSQL for primary relational data. Write SQL explicitly, generate typed Go query methods, and keep versioned SQL migrations with the service that owns the data. Application use cases control business transaction boundaries, and database constraints enforce local invariants.

The selected driver, query generator, and migration runner remain replaceable implementation choices documented in [technology choices](../technology-choices.md).

## Alternatives Considered

### An object-relational mapper

- Pros: less handwritten SQL and convenient object persistence.
- Cons: it hides the SQL and transaction behavior the project is intended to exercise.
- Rejected: explicit queries make database behavior reviewable without requiring manual result mapping.

### Handwritten SQL and handwritten row mapping

- Pros: no query-generation tool in the build.
- Cons: repetitive scanning and type conversion add code without architectural value.
- Rejected: typed generation removes mechanical work while preserving the SQL.

## Consequences

- Schema, migration, query, and generated-code changes remain consistent in one service change.
- Database constraints share responsibility for local integrity with domain rules.
- Application code makes transaction scope explicit.
- Migration ordering, compatibility, and rollback limits require deployment procedures.
