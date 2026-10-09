---
description: Break work into small verifiable tasks and track each task in GitHub
---

Invoke the agent-skills:planning-and-task-breakdown skill. Follow the [module plan conventions](../../docs/conventions/module-plans.md), the [GitHub Issue conventions](../../docs/conventions/github-issues.md), the [GitHub milestone conventions](../../docs/conventions/github-milestones.md), and the [GitHub label conventions](../../docs/conventions/github-labels.md) instead of the skill defaults, as the [differences that the plan convention lists](../../docs/conventions/module-plans.md#differences-from-the-planning-and-task-breakdown-skill) state.

The work starts from one module with an `Approved` specification. It ends when the plan PR has the `Approved` status, every Issue of the plan is in the GitHub Project and the milestone, and CI passes. The maintainer merges the PR. Do not implement a task.

## Prepare

1. Read the specification at `docs/specs/<module-id>.md` and its row in the [specification index](../../docs/specs/README.md). If its status is not `Approved`, stop. Plan the modules in the order of the `Depends on` column.
2. If `tasks/.todo.md` belongs to another module, or another incomplete plan or Issue set exists for this module, stop and ask the maintainer. If `tasks/.todo.md` belongs to this module, update it in place.
3. Read the code that the capability affects. Do not change implementation code.

To change an `Approved` plan, follow the [change rules](../../docs/conventions/module-plans.md#changes) of the plan convention, and use the same steps below.

## Draft

1. Create a branch from `main`, as the [branch name conventions](../../docs/conventions/branch-names.md) state.
2. Use the skill to find the dependencies and to slice the work into tasks. Write the plan as `tasks/<module-id>.md` with the `Draft` status. Write one Issue draft for each task in `tasks/.todo.md`.
3. Show the plan and the drafts to the maintainer in the chat, and wait for their agreement. This agreement does not approve the plan.

## Create the Issues

1. Run `gh auth status --active --hostname github.com` and `gh api user --jq .login`. If either reports a connection or DNS error, retry both outside the sandbox with the current credentials. If GitHub rejects the credentials after a successful connection, ask the maintainer to run `gh auth login -h github.com -p https -w`, and stop.
2. Make sure that the active token has the `project` scope. If it does not, ask the maintainer to run `gh auth refresh -h github.com -s project`, and stop.
3. Search the open Issues for duplicates of the drafts. If one exists, stop and ask the maintainer.
4. Create or reuse the milestone of the plan, as the [milestone workflow](../../docs/conventions/github-milestones.md#workflow) states.
5. Create one Issue from each draft, as the [Issue workflow](../../docs/conventions/github-issues.md#workflow) and the [Issue link rules](../../docs/conventions/github-issues.md#links-and-tracking) state.
6. Make sure that `gh project item-list` shows every new Issue with the `Todo` status, and that `gh issue list --milestone "<milestone title>" --state all --limit 1000` shows every Issue of the plan. Make sure that the `Blocked by` relationships match the dependency graph.

## Open the PR

1. Complete the plan as the [plan workflow](../../docs/conventions/module-plans.md#workflow) states. This step deletes `tasks/.todo.md` and sets the `Approved` status.
2. Commit the plan, as the [commit message conventions](../../docs/conventions/commit-messages.md) state.
3. Push the branch and open the PR, as the [pull request conventions](../../docs/conventions/pull-requests.md) and the label conventions state. Use the PR title from the plan workflow.
4. Wait until every CI check passes.
5. Tell the maintainer in the chat that the PR is ready. Stop.
