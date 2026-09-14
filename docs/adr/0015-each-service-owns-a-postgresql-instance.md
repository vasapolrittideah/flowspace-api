# ADR-0015: Each service owns a PostgreSQL instance

## Status

Accepted

## Date

2026-09-14

## Context

Independent service ownership is weakened if services share tables, credentials, maintenance windows, or recovery operations. A single physical Ubuntu host limits true availability, but database lifecycle and access boundaries can still remain service-owned.

## Decision

Run one PostgreSQL instance per application service in each environment. Each service owns its database, credentials, migrations, connection pool, storage, health checks, backup, and recovery lifecycle. A service never reads another service's database or imports its repository; cross-service access uses synchronous or event contracts.

## Alternatives Considered

### One PostgreSQL instance with a logical database per service

- Pros: fewer processes and lower baseline resource use.
- Cons: maintenance, capacity, credentials, and recovery remain coupled to one database server.
- Rejected: service-level operational ownership is worth the additional instances in this learning environment.

### One shared schema

- Pros: joins and foreign keys work across all application data.
- Cons: any service can bypass another service's rules and schema changes become coordinated.
- Rejected: database access must not erase the service boundary.

## Consequences

- Cross-service identifiers cannot use database foreign keys.
- Services reconcile external references through contracts.
- Each instance needs resource limits, health checks, backups, and restore tests.
- Separate instances still share one physical host and therefore do not provide host-level high availability.
