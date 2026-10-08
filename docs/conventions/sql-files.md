# SQL file conventions

This convention names and groups SQL query files in `services/<service>/db/queries/` and migration files in `services/<service>/db/migrations/`.

[ADR-0014](../adr/0014-relational-data-uses-explicit-sql.md) explains the SQL decision.

A query file holds the queries that sqlc turns into Go methods. A migration file holds 1 schema change that goose applies. The [Hexagonal component and file](hexagonal-components-and-files.md) conventions define a capability and a transaction.

## Rules

### Queries

- Put the related queries of 1 capability or 1 transaction in 1 snake_case file under `services/<service>/db/queries/`, such as `provider_login.sql` or `create_workspace.sql`.
- Put the queries that several capabilities use in a common file, such as `identity.sql`.
- Write each query name in PascalCase. Start it with a verb, such as `GetActiveAccountByEmail`.
- Give each query a name that is unique in the `db/queries/` directory of the service, so that sqlc can generate 1 Go method for each query.
- If a group of related queries of 1 capability changes independently of the other queries in a file, move the group to its own file. Do not split a file by line count, table, RPC method, or individual query.

### Migrations

- Save each migration as `services/<service>/db/migrations/<NNNNN>_<verb>_<object>.sql`. Write the number with 5 digits. Write the verb and the object in snake_case.
- Give each migration the next number in its service. Do not reuse or skip a number.
- Use a verb that states the change, such as `create`, `add`, or `allow`.
- Put the schema changes that must deploy together in 1 migration, including the related tables, indexes, constraints, and data changes. Put independent changes in separate migrations.
- After a migration runs in a shared environment, do not change its file. Add a new migration for a later change.

### Changes

- Apply a change of this convention only to new SQL files and to SQL files that a later PR changes. Do not rename or rewrite other existing SQL files only to follow the change.

## Examples

### Query files

These query files follow the convention:

| File | Content |
| --- | --- |
| `provider_login.sql` | The queries of 1 capability |
| `create_workspace.sql` | The queries of 1 transaction |
| `identity.sql` | The queries that several capabilities use |

### Migration files

These migration files follow the convention:

| File | Change |
| --- | --- |
| `00001_create_identity.sql` | Creates the first Identity tables |
| `00002_add_workspace_creation_expiry.sql` | Adds an expiry to workspace creation records |
