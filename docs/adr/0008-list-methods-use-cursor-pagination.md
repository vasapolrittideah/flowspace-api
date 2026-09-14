# ADR-0008: List methods use bounded cursor pagination

## Status

Accepted

## Date

2026-09-14

## Context

Collection sizes can grow while records are inserted or updated between requests. Returning every record is unbounded, and offset positions can shift under concurrent changes, causing clients to miss or repeat results.

## Decision

Every list method accepts a bounded `page_size` and opaque `page_token` and returns a `next_page_token`. Each collection contract defines a deterministic order plus its default and maximum page sizes. Do not return total counts unless a product use case requires them.

## Alternatives Considered

### Offset and limit pagination

- Pros: simple SQL and human-readable positions.
- Cons: offsets shift during concurrent writes and large offsets require increasing database work.
- Rejected: an opaque cursor preserves the selected order without exposing storage positions.

### Return the complete collection

- Pros: the client makes one request and needs no pagination state.
- Cons: response time and memory grow without a contract limit.
- Rejected: every collection needs a bounded response.

## Consequences

- Implementations maintain a stable deterministic order.
- Tokens are service-owned opaque values rather than client-editable offsets.
- Clients must follow `next_page_token` to continue a listing.
- Total-count queries are paid for only by features that need them.
