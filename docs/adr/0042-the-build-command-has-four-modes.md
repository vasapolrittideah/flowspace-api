# ADR-0042: The build command has `single`, `chain`, `fanout`, and `swarm` modes

Date: 2026-10-09

Status: Accepted

## Context

The maintainer wants 4 ways to run `/build`. A run can implement 1 Issue at a time or start subagents for several Issues at the same time. A run can also stop after its first work or continue with the next work that is ready.

`/build` implements 1 Issue of 1 approved plan and stops. `/build auto` makes the author agent a dispatcher, as [ADR-0040](0040-automatic-builds-run-one-subagent-for-each-ready-issue.md) states. The dispatcher starts 1 set of subagents, waits for them, reports, and ends. Neither run continues after its first work. An Issue that becomes ready during a run waits for the next run, also when its blocker only needed an open PR to start a stack.

ADR-0040 rejected 1 run that loops until the plan is complete. Such a run waits hours or days for each merge, and its context grows for the whole plan. ADR-0040 did not decide whether a run can continue with work that becomes ready without a merge.

## Decision

`/build` takes 1 of 4 modes as its first argument: `single`, `chain`, `fanout`, or `swarm`. Without a mode, it runs `single`. The `auto` argument no longer exists, and `fanout` does what `/build auto` did.

| Mode | Plans | Subagents at the same time | After each subagent ends |
| --- | --- | --- | --- |
| `single` | 1 | None | Not applicable |
| `chain` | 1 | At most 1 | Choose the work again |
| `fanout` | All approved plans | At most 3 | Wait for the others, then end |
| `swarm` | All approved plans | At most 3 | Choose the work again |

In `single` mode, the author agent implements the next ready Issue of 1 plan itself and stops, as `/build` did before this decision. It takes the module ID as its second argument, or uses the only approved plan.

For automatic builds, the author agent is a dispatcher. Each subagent works on 1 Issue or 1 PR. All rules of ADR-0040 and [ADR-0041](0041-maintainers-answer-automatic-builds-in-the-chat.md) apply to these runs. These rules cover the ready Issues, the stacks of at most 2 PRs, and the limit of 3 open or in-progress PRs. They also cover the shared paths, the work that runs alone, the comments, and the lock that allows only 1 dispatcher run at a time.

A `chain` run reads only 1 plan. It takes the module ID as its second argument, or uses the only approved plan. It works on the open PRs of that plan that need work, and then on the ready Issues of that plan in the order of the plan. It starts at most 1 subagent at a time, so the work of the plan goes in sequence, and each Issue starts with a new context.

A `fanout` run does the 3 steps of ADR-0040 and ends.

In `chain` and `swarm` modes, the dispatcher does not end after 1 set of subagents. Each time a subagent ends, the dispatcher reads the Issues and the PRs on GitHub again and chooses the work again within the limits of its mode. An Issue whose blocker got an open PR in the same run can then start as a stacked Issue. Each Issue and each PR gets at most 1 subagent in each run, so a PR that a subagent did not make ready waits for the next run. The run ends when no subagent works and the choice starts no subagent. A run never waits for a merge or polls GitHub for one.

Codex keeps only `single` mode until Codex runs the project hook, as ADR-0040 states for `/build auto`.

## Alternatives Considered

### `auto` as a second name for `fanout`

- Pros: the maintainer can keep the command that they use now.
- Cons: the documents and the run reports are harder to read with 2 names for 1 mode.
- Rejected: the use of 1 name for each mode is clearer, and only the documents that this decision changes use the old name.

### A chain in the author agent without subagents

- Pros: the run needs no dispatcher, and the author agent can ask the maintainer a question in the chat.
- Cons: the context grows with each Issue, and the PRs are not from an automatic build, so they cannot stack and a later automatic build does not fix them.
- Rejected: a run with 1 subagent at a time gives each Issue a new context and uses the same rules as the other automatic builds.

### A run that waits for each merge

- Pros: the run can continue until the plan is complete.
- Cons: the run waits hours or days for each merge, as ADR-0040 states.
- Rejected: a run that ends when it cannot start more work loses nothing, and a scheduled task can start the next run.

## Consequences

- A `chain` or `swarm` run can start more work without a request from the maintainer, but it still stops at the limit of 3 open or in-progress PRs and at stacks of 2 PRs. In a plan where each Issue depends on the one before it, a `chain` run usually ends after 2 Issues.
- A `swarm` run can run longer than a `fanout` run, so it uses more review quota in 1 run and keeps the lock longer.
- The context of the dispatcher grows with each report, but the limit of 1 subagent for each Issue and each PR in a run keeps the number of reports small.
- An answer to a stop that the maintainer gives during a run sets the Project status, but the Issue or the PR does not start again before the next run.
- An unattended `chain` or `swarm` run stops at a permission prompt unless the commands of `/build` are already allowed, as with `/build auto`.
- `.agents/commands/build.md`, the [Agent workflow](../agent-workflow.md), the GitHub Issue conventions, and the Pull request conventions need the 4 modes before the modes exist.
