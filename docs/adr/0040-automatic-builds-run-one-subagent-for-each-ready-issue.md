# ADR-0040: Automatic builds run one subagent for each ready Issue

Date: 2026-10-07

Status: Accepted

## Context

The `/build` command implements one Issue of one approved plan in each run, and the maintainer starts each run. When several Issues of a plan have no open blocker, they wait for the maintainer to start a run for each of them, one at a time. The maintainer wants agents to implement each ready Issue without a request for each one, while the maintainer still reviews and merges every pull request (PR).

The [agent workflow](../agent-workflow.md) keeps one author agent for each change and runs independent tasks in separate worktrees. It does not use the orchestrator-workers pattern, because that pattern selects its subtasks while it works. A plan already defines its tasks as Issues, with blockers and a [Files likely touched](../conventions/github-issues.md#files-likely-touched) section, as the [GitHub Issue](../conventions/github-issues.md) conventions state. A blocker closes only when its PR merges, so an Issue that depends on an open PR cannot start before the maintainer merges that PR.

The `git-guard` hook enforces the review stamps for Claude Code. The agent workflow records that Codex CLI 0.160 skips the project hook, so a Codex agent gets no block for a commit or a PR without approval.

## Decision

In Claude Code, `/build auto` makes the author agent a dispatcher that starts one background subagent for each ready Issue, and each subagent implements its Issue in its own worktree with the steps of `/build`. The dispatcher does not write code, and each subagent is the author agent of one PR.

A ready Issue is an open Issue of a plan with the `Approved` status, with all blockers closed, and with the `Todo` Project status. The dispatcher reads all approved plans.

Before it starts the subagent of an Issue, it sets the Project status of the Issue to `In Progress` and adds an Issue comment that starts with the HTML marker `<!-- automatic-build -->`. The status keeps a later run from starting the same Issue again. The marker tells an automatic build apart from a `/build` run that the maintainer started. Each automatic start adds a new marker comment, and the marker is active only during that attempt. When an attempt stops without a PR, the agent that sets `Needs human` changes the marker of that comment to `<!-- automatic-build-ended -->`.

At most 3 PRs from automatic builds are open or in progress at the same time. Two Issues whose [Files likely touched](../conventions/github-issues.md#files-likely-touched) sections share a path do not run at the same time. An Issue that adds or changes a migration, the final Prove task of a plan, and an Issue whose [Verification](../conventions/github-issues.md#verification) section needs the local cluster each run with no other subagent.

Each run of the dispatcher does three steps and then ends. First, for each open PR from an automatic build with a merge conflict, a failed CI check, or a requested change that the branch does not make yet, it starts one subagent that fixes the PR on its branch. A requested change is a review comment from the maintainer that asks for a change. The squash message comment, a ready notice, and an approval are not requested changes, and a reply without the change does not complete one. Second, it starts the subagents of the ready Issues within the limits above. Third, it waits for the subagents to finish and reports each result.

The maintainer starts the next run, by hand or with a scheduled task. A run does not wait for a merge. Only one run of the dispatcher works at a time, because two runs can both read the `Todo` status of an Issue before either run changes it. A run that finds another run at work stops.

A subagent does not ask the maintainer a question. When a step of `/build` tells the agent to stop and ask, or a review does not pass after 3 rounds, the subagent writes the reason in a comment on the Issue, sets the Project status to `Needs human`, and stops. The dispatcher does not start an Issue with the `Needs human` status. At the start of each run, the dispatcher also sets `Needs human` on each Issue with the `In Progress` status, an active `<!-- automatic-build -->` marker, and no open PR, with a comment that an earlier run ended before it opened a PR. After the maintainer answers the comment, the maintainer sets the status to `Todo`. A later run then starts the Issue again, and its subagent reads the answer.

The subagent posts the squash message in one PR comment, as the [Pull request](../conventions/pull-requests.md) conventions state for a session without a chat. The comment starts with the HTML marker `<!-- squash-message -->` and a sentence that tells the maintainer to paste the message when they squash merge the PR. A collapsed `<details>` block holds the message in a `text` code block. When the message changes, the agent edits this comment and does not post another one. The dispatcher uses the marker to tell this comment apart from a requested change.

The maintainer still approves each specification and plan, and reviews and merges each PR. The review roles, the stamps, the hook, and the ruleset on `main` do not change. Codex keeps `/build` for one Issue in each run, and it gets `/build auto` only after Codex runs the project hook.

## Alternatives Considered

### One run that loops until the plan is complete

- Pros: the maintainer starts the plan once.
- Cons: the run waits hours or days for each merge, and its context grows for the whole plan.
- Rejected: a run that ends after each set of subagents keeps the state on GitHub, so a failed or stopped run loses nothing.

### A separate desktop session for each Issue

- Pros: each session keeps its own PR status and can stay open after the dispatcher ends.
- Cons: only the Claude desktop app can start such sessions, and the maintainer must clean up a session for each Issue.
- Rejected: subagents run in every Claude Code client, and the next run of the dispatcher takes over the work on open PRs.

### Stacked PRs for dependent Issues

- Pros: a dependent Issue can start before its blocker merges.
- Cons: each squash merge of a lower PR forces a rebase of every PR above it.
- Rejected: the limit of 3 open PRs keeps the review queue short, so the wait for a merge costs less than the rebases.

### One scheduled run of `/build` for one Issue

- Pros: it needs no dispatcher and no limits on shared files.
- Cons: Issues without blockers still wait for each other.
- Rejected: subagents in separate worktrees implement independent Issues at the same time with the same reviews.

## Consequences

- The maintainer gets PRs without a request for each Issue, but must review up to 3 PRs at a time.
- Each subagent runs all its review roles, so the use of Codex reviews can triple and reach rate limits sooner.
- A background subagent shows its permission prompts in the main session. An unattended run stops at a prompt unless the commands of `/build` are already allowed.
- A subagent ends after it completes the PR, CI, Issue checkbox, and squash message steps of `/build`, or after it records why it cannot complete them. A later conflict or requested change waits for the next run of the dispatcher.
- An Issue with a wrong [Files likely touched](../conventions/github-issues.md#files-likely-touched) section can still conflict with another open PR, and the next run fixes the conflict.
- An Issue that stops waits with the `Needs human` status until the maintainer answers its comment and sets the status to `Todo`.
- The `Status` field of the GitHub Project needs a `Needs human` option before `/build auto` exists.
- Codex agents cannot use automatic builds until the project hook runs in Codex.
- `.agents/commands/build.md`, the [agent workflow](../agent-workflow.md), the GitHub Issue convention, and the pull request convention need the rules of this decision before `/build auto` exists.

## Sources

- [Building effective agents](https://www.anthropic.com/engineering/building-effective-agents)
- [Create custom subagents](https://code.claude.com/docs/en/sub-agents)
