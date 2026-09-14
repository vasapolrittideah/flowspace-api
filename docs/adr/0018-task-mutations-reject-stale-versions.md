# ADR-0018: Task mutations reject stale versions

## Status

Accepted

## Date

2026-09-14

## Context

Two users can read the same task and submit conflicting edits. Accepting both without a concurrency condition silently overwrites the first committed change and leaves neither caller aware that its view was stale.

## Decision

Require an expected task version for every task content, status, and assignment mutation. Commit only when the stored version matches, increment the version atomically, and return a conflict for a stale value. The client then reloads or deliberately merges before retrying.

## Alternatives Considered

### Last write wins

- Pros: every update succeeds without retry behavior.
- Cons: concurrent changes disappear silently.
- Rejected: collaboration must expose conflicts instead of choosing by arrival order.

### Hold a pessimistic lock while a user edits

- Pros: another writer cannot commit over the active editor.
- Cons: user think time creates long locks, expiry rules, and abandoned-edit recovery.
- Rejected: optimistic conflicts are cheaper for short database mutations and human-paced editing.

## Consequences

- Task responses expose a version that mutation requests return.
- Database updates include the expected version in their atomic condition.
- Clients handle conflicts as a normal outcome.
- Merging remains an explicit product behavior rather than an implicit server guess.
