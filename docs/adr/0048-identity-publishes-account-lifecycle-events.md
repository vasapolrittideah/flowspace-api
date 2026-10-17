# ADR-0048: Identity publishes account lifecycle events

Date: 2026-10-17

Status: Accepted

## Context

A user will delete their Flowspace account. Identity keeps the account for a 30-day grace period, lets the user restore it in that period, and then deletes its personal data and keeps only the subject. Workspace references the subject in memberships, and Workspace must react when an account leaves, for example when the account is the only owner of a workspace.

[ADR-0016](0016-cross-service-side-effects-use-domain-events.md) integrates cross-service side effects through versioned domain events, and [ADR-0017](0017-outbox-and-idempotent-consumers-deliver-events.md) delivers them through an outbox and idempotent consumers. Today only Workspace and Work publish events for other services. Identity publishes only `EmailDeliveryRequested` to a private topic for its own mail worker, as [ADR-0035](0035-identity-signup-security-and-mail-delivery.md) states. The [architecture](../architecture.md#service-boundaries) states that Workspace depends on Identity only for access tokens and session checks.

## Decision

Identity publishes account lifecycle events for other Flowspace services on the topic `identity.account-lifecycle.v1`. The first consumer is Workspace.

Identity publishes these events:

- `AccountDeletionScheduled`, when a user requests the deletion of their account.
- `AccountRestored`, when the user restores the account in the grace period.
- `AccountDeleted`, when Identity deletes the personal data of the account at the end of the grace period.

Each event carries its event ID, the subject, the time of the fact, and for `AccountDeletionScheduled`, the time of the planned deletion. An event carries no email address, name, provider subject, or other personal data. The subject is the only account reference.

Identity commits each event with its account change in 1 transaction through its outbox, as ADR-0017 states, and publishes with the subject as the record key. Consumers then receive the events of 1 subject in the commit order of its account changes, also when the publisher retries. Each consumer applies an event idempotently by its event ID.

Identity never waits for a consumer. A consumer failure delays its own effect and does not block or undo a deletion or a restore in Identity. Each consumer defines its own effects in its own specification.

The topic is separate from the private email delivery topic of ADR-0035. Mail workers do not read it, and other services do not read the email delivery topic.

## Alternatives Considered

### A synchronous call from Identity to Workspace

- Pros: Identity learns the result of Workspace at once and can refuse a deletion.
- Cons: Identity depends on the availability of Workspace, and a new service needs a change in Identity.
- Rejected: an event keeps Identity independent of its consumers, and the maintainer chose to allow a deletion in every case.

### Workspace reads deleted accounts from Identity

- Pros: Identity publishes nothing new, and Workspace asks for the state when it needs it.
- Cons: Workspace must poll Identity or check each subject on use, and it misses the time of each change.
- Rejected: a pushed event tells each consumer about each change in order.

### Personal data in the events

- Pros: a consumer can show the email or name of a deleted member without another lookup.
- Cons: the events copy personal data into the broker and into each consumer, and the deletion must then reach every copy.
- Rejected: the subject is enough for a consumer to find its own records, and the deletion stays in Identity.

## Consequences

- Workspace depends on Identity events as well as on access tokens and session checks. The service boundaries and the diagram of the architecture show this dependency.
- Identity needs a public event contract with a registered schema and a version, as [ADR-0006](0006-version-events-separately-from-rpc.md) states.
- Event retention, consumer deduplication, and the replay window must cover a consumer outage, as the architecture requires for every event stream.
- A consumer can apply `AccountDeleted` after the subject no longer has personal data in Identity, because the event carries only the subject.
- The Identity account deletion specification defines the events, the grace period, the restore, and the deletion. The effects in Workspace need their own specification.
