# ADR-0023: Hosted CI stays within the zero-spend limit

## Status

Accepted

## Date

2026-09-14

## Context

The public repository needs reproducible review checks without maintaining another CI server or consuming resources from the single application host. The project has a strict zero-spend limit for hosted automation.

## Decision

Host source and reviews in the public GitHub repository and run checks on hosted CI only while they remain within the zero-spend limit. Keep repository checks reproducible locally. Do not operate a self-hosted runner on the application server.

## Alternatives Considered

### A self-hosted runner on the application server

- Pros: no hosted-minute limit and direct control over the execution environment.
- Cons: builds compete with application workloads and give repository automation a path onto the server.
- Rejected: CI must not consume or expand trust on the constrained host.

### A separate paid CI service

- Pros: independent capacity and additional workflow features.
- Cons: it violates the project's zero-spend constraint.
- Rejected: current checks fit the available hosted allowance.

## Consequences

- CI workload must remain inside the free hosted allowance.
- Required checks also have local commands for development and recovery.
- Application-host capacity is reserved for the deployed learning system.
- Exceeding the allowance requires a new cost or runner decision.
