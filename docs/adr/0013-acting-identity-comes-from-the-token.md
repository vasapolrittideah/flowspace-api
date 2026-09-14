# ADR-0013: Acting identity comes from the validated token

## Status

Accepted

## Date

2026-09-14

## Context

Requests may contain resource owners, assignees, or membership subjects, but none of those fields prove who is making the request. Trusting an actor supplied in a body, path, or forwarded client-controlled header would let callers choose the identity used for authorization and audit.

## Decision

Derive the acting identity from the subject of the validated token. Request bodies and paths may identify resources or target subjects but never supply trusted actor or role claims. Propagate the authenticated identity explicitly through the REST proxy and service boundary so application authorization and audit use the same principal.

## Alternatives Considered

### Accept an actor identifier in the request

- Pros: simple local testing and explicit handler inputs.
- Cons: a caller can impersonate another subject unless every handler duplicates a comparison with the token.
- Rejected: authenticated identity is transport context, not client-authored resource data.

### Trust a client-controlled role claim

- Pros: services can authorize without loading workspace membership.
- Cons: the caller can assert permissions it does not own.
- Rejected: application roles come from Workspace-owned authorization state.

## Consequences

- Handlers must carry authenticated context into application code.
- Audit records use the validated subject consistently.
- Resource ownership fields remain separate from the actor performing an operation.
- Tests provide authenticated context rather than placing an actor in request data.
