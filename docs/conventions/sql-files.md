# SQL file conventions

This convention names and groups SQL query and migration files inside each service. [ADR-0014](../adr/0014-relational-data-uses-explicit-sql.md) explains the SQL decision.

## Rules

### Queries

- Group related queries by capability or transaction in a snake_case file under the owning service's `db/queries/`, such as `password_recovery.sql`.
- Keep queries used by several capabilities in a common file, such as `identity.sql`. Split the file when a distinct capability has a group of related queries that changes independently. Do not split by line count, table, RPC method, or individual query.
- Keep query names unique within the service's `db/queries/` directory so sqlc can generate the Go methods.

### Migrations

- Put schema changes that must deploy together in one numbered file under the owning service's `db/migrations/`. Include related tables, indexes, constraints, and data changes in that file. Put independent changes in separate files.
- Name new files `<number>_<verb>_<object>.sql` in snake_case. Use a verb that describes the change, such as `00002_add_workspace_creation_expiry.sql`.
- Once a migration runs in a shared environment, leave its file unchanged. Add a new numbered migration for later changes.
