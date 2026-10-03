# SQL file conventions

This convention names and groups the SQL query files and migration files inside each service. A query file holds the queries that sqlc turns into Go methods. A migration file holds one schema change that goose applies. [ADR-0014](../adr/0014-relational-data-uses-explicit-sql.md) explains the SQL decision. The [hexagonal component and file conventions](hexagonal-components-and-files.md) define a capability and a transaction.

## Rules

### Queries

- Put the related queries of one capability or one transaction in one snake_case file under `services/<service>/db/queries/`, such as `provider_login.sql` or `create_workspace.sql`.
- Put the queries that several capabilities use in a common file, such as `identity.sql`.
- Write each query name in PascalCase, and start it with a verb, such as `GetActiveAccountByEmail`.
- Give each query a name that is unique in the `db/queries/` directory of the service, so that sqlc can generate one Go method for each query.
- If a group of related queries of one capability changes independently of the other queries in a file, move the group to its own file. Do not split a file by line count, table, RPC method, or individual query.

### Migrations

- Save each migration as `services/<service>/db/migrations/<NNNNN>_<verb>_<object>.sql`. Write the number with five digits, and write the verb and the object in snake_case.
- Give each migration the next number in its service. Do not reuse or skip a number.
- Use a verb that states the change, such as `create`, `add`, or `allow`.
- Put the schema changes that must deploy together in one migration, including the related tables, indexes, constraints, and data changes. Put independent changes in separate migrations.
- After a migration runs in a shared environment, do not change its file. Add a new migration for a later change.

### Changes

- Apply a change of this convention to new files, and to query files that a later PR changes. Do not rename or rewrite a migration file to follow it.

## Examples

### Query files

These query files follow the convention:

| File | Content |
| --- | --- |
| `provider_login.sql` | The queries of one capability |
| `create_workspace.sql` | The queries of one transaction |
| `identity.sql` | The queries that several capabilities use |

### Migration files

These migration files follow the convention:

| File | Change |
| --- | --- |
| `00001_create_identity.sql` | Creates the first Identity tables |
| `00002_add_workspace_creation_expiry.sql` | Adds an expiry to workspace creation records |
