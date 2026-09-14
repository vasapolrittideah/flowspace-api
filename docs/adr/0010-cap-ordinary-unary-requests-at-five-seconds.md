# ADR-0010: Ordinary unary requests are capped at five seconds

## Status

Accepted

## Date

2026-09-14

## Context

A caller needs a bounded answer when a service or dependency is slow. Allowing requests to wait indefinitely consumes shared resources and makes an ambiguous timeout policy differ across transports and handlers.

## Decision

Honor a caller deadline when it is shorter and cap ordinary unary requests at five seconds. Propagate the effective deadline through the REST proxy and synchronous service calls. An operation that cannot reliably finish within that bound requires a separately designed asynchronous or long-running contract rather than a larger implicit timeout.

## Alternatives Considered

### No service-side cap

- Pros: long operations can finish without a new API shape.
- Cons: abandoned callers and stalled dependencies can retain resources indefinitely.
- Rejected: ordinary request latency must have a predictable upper bound.

### A longer default timeout for every request

- Pros: fewer operations reach a deadline during temporary slowness.
- Cons: it delays failure detection and applies long-running needs to every method.
- Rejected: exceptional operations should expose exceptional behavior explicitly.

## Consequences

- Callers can choose a deadline shorter than five seconds.
- Downstream work should stop when the effective deadline expires.
- Operations exceeding the cap need a new contract decision.
- The five-second value must be measured and superseded if normal workloads cannot meet it.
