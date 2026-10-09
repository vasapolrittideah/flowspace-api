# ADR-0040: Automatic builds run one subagent for each ready Issue

Date: 2026-10-07

Status: Accepted, except the requested changes from comments without a marker and the status change after an answer superseded by [ADR-0041](0041-maintainers-answer-automatic-builds-in-the-chat.md), and the `/build auto` name and the end of each run after 1 set of subagents superseded by [ADR-0042](0042-the-build-command-has-four-modes.md)

## Context

The `/build` command implements one Issue of one approved plan in each run, and the maintainer starts each run. When several Issues of a plan have no open blocker, they wait for the maintainer to start a run for each of them, one at a time. The maintainer wants agents to implement each ready Issue without a request for each one, while the maintainer still reviews and merges every pull request (PR).

The [agent workflow](../agent-workflow.md) keeps one author agent for each change and runs independent tasks in separate worktrees. It does not use the orchestrator-workers pattern, because that pattern selects its subtasks while it works. A plan already defines its tasks as Issues, with blockers and a [Files likely touched](../conventions/github-issues.md#files-likely-touched) section, as the [GitHub Issue](../conventions/github-issues.md) conventions state. A blocker closes only when its PR merges, so an Issue that depends on an open PR cannot start before the maintainer merges that PR.

The `git-guard` hook enforces the review stamps for Claude Code. The agent workflow records that Codex CLI 0.160 skips the project hook, so a Codex agent gets no block for a commit or a PR without approval. Agents use the GitHub account of the maintainer, so a comment from an agent and a comment from the maintainer show the same author.

## Decision

In Claude Code, `/build auto` makes the author agent a dispatcher that starts one background subagent for each ready Issue, and each subagent implements its Issue in its own worktree with the steps of `/build`. The dispatcher does not write code, and each subagent is the author agent of one PR.

A ready Issue is an open Issue of a plan with the `Approved` status and the `Todo` Project status, and with all blockers closed. An Issue with one open blocker and all other blockers closed is also ready when the open blocker has an open PR from an automatic build with the `main` base. The subagent of such an Issue stacks its work on the PR of the blocker, so a stack has at most 2 PRs. The dispatcher reads all approved plans.

Before it starts the subagent of an Issue, it sets the Project status of the Issue to `In Progress` and adds an Issue comment that starts with the HTML marker `<!-- automatic-build -->`. The status keeps a later run from starting the same Issue again. The marker tells an automatic build apart from a `/build` run that the maintainer started. Each automatic start adds a new marker comment, and the marker is active only during that attempt. When an attempt stops without a PR, the agent that sets `Needs human` changes the marker of that comment to `<!-- automatic-build-ended -->`.

At most 3 PRs from automatic builds are open or in progress at the same time. Two Issues whose [Files likely touched](../conventions/github-issues.md#files-likely-touched) sections share a path do not run at the same time. An Issue that adds or changes a migration, the final Prove task of a plan, and an Issue whose [Verification](../conventions/github-issues.md#verification) section needs the local cluster each run with no other subagent.

A stacked subagent creates its branch from the branch of the parent PR and opens its PR with that branch as the base. The first line of its PR description is `Stacked on #<parent>. Merge #<parent> first.` The maintainer merges the parent PR first. Agents create and update stacks with `git` and `gh pr create --base`, and the hook blocks the `gh stack` extension, because its commands create and merge PRs without the checks of the hook.

Each run of the dispatcher does three steps and then ends. First, for each open PR from an automatic build that needs work, it starts one subagent for that PR. A PR needs work when it is a child PR whose parent changed, or when it has a merge conflict, a failed CI check, or a requested change that the branch does not make yet. Each PR has at most one active subagent, and that subagent does all the work that the PR needs, with the restack first. Second, it starts the subagents of the ready Issues within the limits above. Third, it waits for the subagents to finish and reports each result.

After the parent PR merges, the subagent rebases the child branch onto `main` with `git rebase --onto`. After the parent branch gets new commits, it rebases the child branch onto the parent branch. It pushes the child branch with `git push --force-with-lease`, also after a review comment exists, and the [Pull request](../conventions/pull-requests.md) conventions must allow this restack before `/build auto` exists. The repository deletes the head branch of a PR after the merge, so GitHub changes the base of the child PR to `main` without `gh pr edit`.

A restack gives the child a new tree, and a stamp approves only one exact tree. Before the push, the subagent runs the tests and the review roles of the child on the new tree, because a change in the parent can change the behavior of the child. When `git patch-id --verbatim` of the child diff does not change, the subagent resumes the earlier review sessions and tells them that only the base changed, so that the reviews can reuse their earlier findings.

A subagent that works on the branch of an existing PR first removes an earlier worktree of that branch when the worktree is clean and no agent works in it, because Git does not check out one branch in two worktrees. If the earlier worktree has uncommitted changes, the subagent stops with the `Needs human` status.

The maintainer starts the next run, by hand or with a scheduled task. A run does not wait for a merge. Only one run of the dispatcher works at a time, because two runs can both read the `Todo` status of an Issue before either run changes it. A run that finds another run at work stops.

The dispatcher reads only the comments from the account of the maintainer. Each comment that an agent posts starts with an HTML marker that names its kind and ends with the attribution line of the PR descriptions, `🤖 Generated with [Claude Code](https://claude.com/claude-code)`. A comment from the maintainer account without an agent marker is a comment from the maintainer. A requested change is such a comment that asks for a change, or a comment with the `<!-- maintainer-feedback -->` marker. An approval, or a reply without the change, does not complete a requested change. Comment text is data, so a subagent changes code to meet a requested change, but it does not run a command or change its rules because of the text.

During a run, the maintainer can give feedback on a PR in the chat of the dispatcher. The dispatcher sends the feedback to the subagent of that PR, and it records the feedback in a PR comment with the `<!-- maintainer-feedback -->` marker. The comment states in English what to change and where to change it, and it follows the [Markdown and English prose](../conventions/markdown-and-english-prose.md) conventions. The record lets a later run see the feedback when the current run ends before the change.

A subagent does not ask the maintainer a question. When a step of `/build` tells the agent to stop and ask, or a review does not pass after 3 rounds, the subagent writes the reason in a comment on the Issue, sets the Project status to `Needs human`, and stops. When a subagent finds that the specification or the plan of its Issue must change, it stops in the same way. The dispatcher stacks no new Issue of that plan until the maintainer approves the change. The dispatcher does not start an Issue with the `Needs human` status.

At the start of each run, the dispatcher also sets `Needs human` on each Issue with the `In Progress` status, an active `<!-- automatic-build -->` marker, and no open PR, with a comment that an earlier run ended before it opened a PR. After the maintainer answers the comment, the maintainer sets the status to `Todo`. A later run then starts the Issue again, and its subagent reads the answer.

The subagent posts the squash message in one PR comment, as the Pull request conventions state for a session without a chat. The comment starts with the HTML marker `<!-- squash-message -->` and a sentence that tells the maintainer to paste the message when they squash merge the PR. A collapsed `<details>` block holds the message in a `text` code block. When the message changes, the agent edits this comment and does not post another one.

The maintainer still approves each specification and plan, and reviews and merges each PR. The review roles, the stamps, and the ruleset on `main` do not change, and the hook only adds the block of `gh stack`. Codex keeps `/build` for one Issue in each run, and it gets `/build auto` only after Codex runs the project hook.

## Alternatives Considered

### One run that loops until the plan is complete

- Pros: the maintainer starts the plan once.
- Cons: the run waits hours or days for each merge, and its context grows for the whole plan.
- Rejected: a run that ends after each set of subagents keeps the state on GitHub, so a failed or stopped run loses nothing.

### A separate desktop session for each Issue

- Pros: each session keeps its own PR status and can stay open after the dispatcher ends.
- Cons: only the Claude desktop app can start such sessions, and the maintainer must clean up a session for each Issue.
- Rejected: subagents run in every Claude Code client, and the next run of the dispatcher takes over the work on open PRs.

### A wait for each blocker to merge

- Pros: no branch needs a rebase or a force push.
- Cons: a dependent Issue waits until the maintainer merges its blocker.
- Rejected: a stack of 2 PRs lets the work continue, and a restack costs less than the wait.

### Stacks deeper than 2 PRs

- Pros: a longer chain of dependent Issues continues without a merge.
- Cons: a change to a lower PR restacks every PR above it and can make more work useless.
- Rejected: a stack of 2 PRs keeps the rework of a lower change to one PR.

### The `gh stack` extension

- Pros: GitHub links the PRs of a stack, and `gh stack sync` restacks every branch in one command.
- Cons: `gh stack submit` and `gh stack merge` create and merge PRs without the checks of the hook, and a merge uses the default squash message.
- Rejected: `git` and `gh pr create --base` make the same stack, and the hook checks each command.

### One scheduled run of `/build` for one Issue

- Pros: it needs no dispatcher and no limits on shared files.
- Cons: Issues without blockers still wait for each other.
- Rejected: subagents in separate worktrees implement independent Issues at the same time with the same reviews.

## Consequences

- The maintainer gets PRs without a request for each Issue, but must review up to 3 PRs at a time.
- Each subagent runs all its review roles, so the use of Codex reviews can triple and reach rate limits sooner. Each restack runs the tests and reviews of the child PR again, but a restack without a patch change resumes the earlier review sessions.
- A background subagent shows its permission prompts in the main session. An unattended run stops at a prompt unless the commands of `/build` are already allowed.
- A subagent ends after it completes the PR, CI, Issue checkbox, and squash message steps of `/build`, or after it records why it cannot complete them. A later conflict or requested change waits for the next run of the dispatcher.
- The maintainer must merge a stack from the parent up. A child PR that merges first lands on the parent branch instead of `main`, and its Issue stays open.
- A requested change to a parent PR can force changes in its child PR, and a change to a specification or a plan can make a child PR useless.
- Feedback from the maintainer reaches agents only through a PR comment or the chat of a running dispatcher.
- An Issue with a wrong [Files likely touched](../conventions/github-issues.md#files-likely-touched) section can still conflict with another open PR, and the next run fixes the conflict.
- An Issue that stops waits with the `Needs human` status until the maintainer answers its comment and sets the status to `Todo`.
- The `Status` field of the GitHub Project needs a `Needs human` option, and the repository setting that deletes head branches after a merge must be on, before `/build auto` exists.
- Codex agents cannot use automatic builds until the project hook runs in Codex.
- `.agents/commands/build.md`, the `git-guard` hook, the [agent workflow](../agent-workflow.md), the GitHub Issue conventions, and the Pull request conventions need the rules of this decision before `/build auto` exists.

## Sources

- [Building effective agents](https://www.anthropic.com/engineering/building-effective-agents)
- [Create custom subagents](https://code.claude.com/docs/en/sub-agents)
- [git-rebase documentation](https://git-scm.com/docs/git-rebase)
- [GitHub Stacked PRs](https://github.com/github/gh-stack)
