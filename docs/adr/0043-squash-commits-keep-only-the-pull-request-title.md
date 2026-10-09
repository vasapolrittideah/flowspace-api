# ADR-0043: Squash commits keep only the pull request title

Date: 2026-10-09

Status: Accepted, except the Review notes section superseded by [ADR-0044](0044-pull-request-descriptions-have-no-review-notes-section.md)

## Context

The maintainer squash merges each pull request (PR), as the [Repository instructions](../../AGENTS.md) state. For each PR, the author agent writes a squash message, makes sure that it follows the conventions with `scripts/check-pr-metadata.mjs`, and runs 2 review roles on it. Then it gives the message in the chat, or a subagent posts it in a PR comment, as [ADR-0040](0040-automatic-builds-run-one-subagent-for-each-ready-issue.md) states. The maintainer pastes the message into GitHub at each merge. Each PR needs this work, and a wrong paste puts a wrong message on `main`.

The [Commit messages](../conventions/commit-messages.md) conventions take the squash body from the [What changed](../conventions/pull-requests.md#what-changed) and [Why](../conventions/pull-requests.md#why) sections of the PR description and rewrite it as plain text. The body adds no fact that the PR does not hold, and many squash commits have no body. Each squash commit links to its PR through the `(#<number>)` suffix that GitHub adds to the subject. On 2026-10-09, the default squash message of the repository became the PR title with a blank body. When the maintainer tested this configuration, GitHub still added the `Co-authored-by` trailers of the checkpoint commits to the squash commit. GitHub closes each Issue that a `Closes #<number>` line in the PR description names when the PR merges into `main`.

The [Pull requests](../conventions/pull-requests.md) conventions write the PR description for reviewers. The description has no section for the changes that callers must make after a breaking change, or for the commit that a revert undoes. It also puts the text that only reviewers need next to the text that a later reader of `main` needs.

## Decision

Each squash commit uses only the PR title as its subject. The PR description holds the lasting record of the change, and agents do not write a squash message.

GitHub adds the `(#<number>)` suffix and copies the `Co-authored-by` trailers from the checkpoint commits. The default squash message of the repository uses the PR title and a blank body. In the GitHub API, the `squash_merge_commit_title` value is `PR_TITLE`, and the `squash_merge_commit_message` value is `BLANK`. The maintainer merges with this default and does not paste a message. Each checkpoint commit keeps the `Co-authored-by` trailers that the Commit messages conventions require, because the squash commit gets its trailers from them.

The PR description has 7 sections in 2 groups. The first group is the lasting record for a later reader of `main`. The second group is for the reviewer and the merge decision.

| Order | Section | Group | Content |
| --- | --- | --- | --- |
| 1 | What changed | Lasting record | The lasting effect on `main` first, then each change that a reviewer must know. |
| 2 | Why | Lasting record | The problem, the reason for the change, and a link to each decision that it depends on. |
| 3 | Breaking changes | Lasting record | What breaks and what callers must change, or `n/a`. |
| 4 | Related issues | Lasting record | A `Closes` or `Refs` line for each Issue, or `n/a`. |
| 5 | Review notes | Reviewer | The reading order, the size, the parts that need review, the parts left out on purpose, and the related PRs, or `n/a`. |
| 6 | Risks or limitations | Reviewer | Revert safety, unresolved failures, checks that did not run, and remaining limits, or `n/a`. |
| 7 | Follow-up tasks | Reviewer | The work for later, or `n/a`. |

The Breaking changes section moves the compatibility effects out of [Risks or limitations](../conventions/pull-requests.md#risks-or-limitations). When the PR title has a breaking marker, the section must not be `n/a`. When a PR changes more than 1 value, such as a configuration value, a default, a limit, or an API field, [What changed](../conventions/pull-requests.md#what-changed) shows the values in a table with `Before` and `After` columns. For a revert, [What changed](../conventions/pull-requests.md#what-changed) starts with the full SHA of the reverted commit, and [Why](../conventions/pull-requests.md#why) gives the reason. [Risks or limitations](../conventions/pull-requests.md#risks-or-limitations) states revert safety only when a revert of the PR is not safe, such as after a migration.

Review notes holds only the text that helps a reviewer read the diff. A small PR, such as a PR that a reviewer can read in 1 pass, has `n/a` there. After the PR merges, an agent edits the description only to fix a wrong fact.

`scripts/check-pr-metadata.mjs` makes sure that the Breaking changes section is not `n/a` when the title has a breaking marker. It also makes sure that the [What changed](../conventions/pull-requests.md#what-changed) section starts with a commit SHA when the title has the `revert` type.

This decision replaces 1 part of ADR-0040: the squash message comment of a subagent. A subagent reports a ready PR without that comment.

## Alternatives Considered

### A squash body that the agent writes

- Pros: `git log` shows the reason for each change without GitHub.
- Cons: each PR needs a written message, 2 reviews of it, and a paste by the maintainer, and the body repeats facts from the PR description.
- Rejected: the `(#<number>)` suffix links each commit to the PR that holds the same facts, so the copy adds work and no information.

### The PR title and description as the default squash message

- Pros: GitHub fills the message, and `git log` shows the reasons.
- Cons: the Markdown, the review notes, the risks, and the follow-up tasks of the description go into the history of `main`.
- Rejected: a blank body keeps the history of `main` short, and the PR keeps the full description.

### A squash message set when auto-merge starts

- Pros: GitHub uses the message of the agent, and the maintainer pastes nothing.
- Cons: auto-merge merges a PR without a last look from the maintainer, and the Repository instructions do not let agents start it.
- Rejected: a blank body removes the paste and keeps the merge with the maintainer.

### A squash body only for a PR that needs one

- Pros: `git log` keeps the reasons for breaking changes and migrations.
- Cons: the maintainer still pastes a message for those PRs, and agents follow 2 rules for squash commits.
- Rejected: the PR description holds the same facts, and 1 rule for each PR is simpler.

## Consequences

- `git log` and `git blame` show only the subject of a squash commit. A reader opens the PR on GitHub to read the reasons.
- The maintainer merges with the default message and does not paste text. A wrong default does not show in the repository files, so a change of the configuration on GitHub changes each later squash commit.
- The PR title goes to `main` as it is, so the title checks of `scripts/check-pr-metadata.mjs` and the review roles still apply before the merge.
- An edit to a PR description after the merge changes the lasting record. Agents edit it only to fix a wrong fact.
- Squash commits on `main` from before this decision keep their bodies.
- The Commit messages and Pull requests conventions, the PR template, `scripts/check-pr-metadata.mjs`, the convention-reviewer and writing-reviewer roles, the Review roles rules, the `/build`, `/plan`, and `/spec` commands, the "squash message" term in the glossary, and the Repository instructions need these rules in a later PR.

## Sources

- [Configuring commit squashing for pull requests](https://docs.github.com/en/repositories/configuring-branches-and-merges-in-your-repository/configuring-pull-request-merges/configuring-commit-squashing-for-pull-requests)
- [Linking a pull request to an issue](https://docs.github.com/en/issues/tracking-your-work-with-issues/using-issues/linking-a-pull-request-to-an-issue)
