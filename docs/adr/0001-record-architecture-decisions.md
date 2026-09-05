# ADR-0001: Record architecture decisions

- **Status:** Accepted
- **Date:** 2026-09-05
- **Deciders:** Vasapol Rittideah
- **Tags:** `delivery`

## Context

FlowSpace is a single-maintainer learning project that deliberately adopts
enterprise-grade patterns (ReBAC, transactional outbox, saga orchestration,
GitOps, dynamic secrets). Most of the cost of these patterns is paid at the
moment of choosing them, and the reasoning is invisible in the resulting code.

Six months from now the maintainer will not remember why Redpanda was chosen
over Kafka, why there is a single `go.mod`, or why there is no service mesh —
only that changing any of them looks easy and turns out not to be.

## Decision

We record every architecturally significant decision as a numbered Markdown file
in `docs/adr/`, using `template.md`.

A decision is architecturally significant if reversing it would require changing
more than one service, a data migration, or a redeployment of platform
infrastructure.

- Filename: `NNNN-kebab-case-title.md`, numbers assigned sequentially, never
  reused.
- The Context, Decision and Consequences of an Accepted ADR are immutable. To
  change a decision, write a new ADR and set the old one to
  `Superseded by ADR-NNNN`. Status and cross-reference links are metadata, not
  decisions, and are maintained in place — otherwise a supersession could never
  be recorded in the ADR it supersedes.
- An ADR cites another by number only if that ADR already exists. A decision
  not yet taken is named by its subject ("the migration tool", "the backup
  procedure"), never by a number reserved in advance: reserved numbers drift as
  the order of work changes, and a link to the wrong ADR is worse than no link.
  References point backwards. A new ADR links to the decisions it builds on;
  earlier ADRs are not revisited to point forward at it, which would turn every
  new decision into an edit of every decision before it.
- New ADRs land in the same pull request as the first code that depends on them.
- Prose language is English; the reasoning may be drafted in any language but is
  committed in English so the file stays greppable alongside the code.

## Consequences

### Positive

- Decisions are reviewable in a pull request, like code.
- `git log docs/adr/` becomes the project's architectural history.
- Onboarding (including future-self) reads a directory instead of a codebase.

### Negative / accepted costs

- Writing an ADR takes ~30 minutes and delays the first line of code.
- ADRs rot if the "Revisit when" section is never checked. Accepted: rot is
  visible here, whereas undocumented reasoning is not.

### Neutral

- Adds one directory and no tooling. A new ADR is a `cp` of `template.md`.

## Alternatives considered

| Option                      | Why not                                                           |
| --------------------------- | ----------------------------------------------------------------- |
| Wiki / Notion page          | Drifts from the code, not reviewed, not versioned with the change |
| Long-form `ARCHITECTURE.md` | One growing file with no history of what was rejected or when     |
| Nothing                     | The default failure mode this ADR exists to prevent               |

## Revisit when

Never, unless the ADR directory exceeds ~50 files and needs an index or tooling
(for example `adr-tools`). Until then, `ls docs/adr/` is the index.

## References

- Michael Nygard, *Documenting Architecture Decisions* (2011).
- [MADR — Markdown Any Decision Records](https://adr.github.io/madr/)
