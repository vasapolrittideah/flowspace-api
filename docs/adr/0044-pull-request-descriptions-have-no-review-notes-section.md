# ADR-0044: Pull request descriptions have no Review notes section

Date: 2026-10-09

Status: Accepted

## Context

[ADR-0043](0043-squash-commits-keep-only-the-pull-request-title.md) gives the pull request (PR) description 7 sections. One of them is Review notes, which holds the reading order, the size, the parts that need review, the parts left out on purpose, and the related PRs. A PR that a reviewer can read in 1 pass has `n/a` there.

Most PRs of the repository change a few files or only documents. ADR-0043 gives such a PR `n/a` in Review notes, so the section adds 1 more `n/a` line to most PRs. Each review role reads the whole diff, so a reading order helps only the maintainer, and only on a large PR.

## Decision

The PR description has no Review notes section. This decision replaces the Review notes section of ADR-0043 and keeps the other 6 sections in the same order: [What changed](../conventions/pull-requests.md#what-changed), [Why](../conventions/pull-requests.md#why), [Breaking changes](../conventions/pull-requests.md#breaking-changes), [Related issues](../conventions/pull-requests.md#related-issues), [Risks or limitations](../conventions/pull-requests.md#risks-or-limitations), and [Follow-up tasks](../conventions/pull-requests.md#follow-up-tasks).

`scripts/check-pr-metadata.mjs` requires these 6 headings. The rules of ADR-0043 for the other sections do not change.

## Alternatives Considered

### A Review notes section, as ADR-0043 states

- Pros: the maintainer gets a reading order and the parts that need review on a large PR.
- Cons: ADR-0043 gives most PRs `n/a` there, and the review roles read the whole diff without it.
- Rejected: the maintainer does not need the section on most PRs, and 6 sections keep the description shorter.

### Review notes merged into Risks or limitations

- Pros: the template has 6 sections and keeps the reading order, because 1 section holds all text for the reviewer.
- Cons: [Risks or limitations](../conventions/pull-requests.md#risks-or-limitations) gets a second purpose, the reading guide, next to the risks of the merge.
- Rejected: the maintainer keeps [Risks or limitations](../conventions/pull-requests.md#risks-or-limitations) as it is and does not need the reading order.

## Consequences

- A large PR has no fixed place for a reading order. The maintainer reads the diff in the order that GitHub shows.
- The PR template, the Pull requests conventions, and `scripts/check-pr-metadata.mjs` drop the section in the same PR as this record.
