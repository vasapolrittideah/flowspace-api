---
description: Implement the next GitHub Issue of a plan with tests, commits, and verification
---

Invoke the agent-skills:incremental-implementation skill and the agent-skills:test-driven-development skill. If a step fails, follow the agent-skills:debugging-and-error-recovery skill.

The work starts from one module with an `Approved` plan. It implements one task, which is the next ready Issue of the plan. It ends when the PR of that task is ready, the Issue shows its results, and the maintainer has the squash message. The maintainer merges the PR. Do not start the next task in the same run.

## Select the task

1. Use `$ARGUMENTS` as the module ID. If it is empty, use the only plan in `tasks/` with the `Approved` status. If there is no such plan, or there is more than one, stop and ask for the module ID.
2. Run `gh auth status --active --hostname github.com` and `gh api user --jq .login`. If either reports a connection or DNS error, retry both outside the sandbox with the current credentials. If GitHub rejects the credentials after a successful connection, ask the maintainer to run `gh auth login -h github.com -p https -w`, and stop.
3. Make sure that the active token has the `project` scope. If it does not, ask the maintainer to run `gh auth refresh -h github.com -s project`, and stop.
4. For each closed Issue of the plan, make sure that its Project status is `Done`, as the [Issue workflow](../../docs/conventions/github-issues.md#workflow) states. If the plan is `Complete`, close its milestone as the [milestone workflow](../../docs/conventions/github-milestones.md#workflow) states, and stop.
5. If `docs/specs/<module-id>.md` is not `Approved`, the plan is not `Approved`, or `tasks/.todo.md` exists, stop.
6. Select the first open Issue in the order of the plan whose blockers are all closed. Read the blockers with `gh api repos/{owner}/{repo}/issues/<issue-number>/dependencies/blocked_by`. If no Issue is ready, report the blockers and stop.

## Implement

1. Run `git status --porcelain`, and preserve work outside the task.
2. Create a branch from `main`, as the [branch name conventions](../../docs/conventions/branch-names.md) state.
3. Set the Project status of the Issue to `In Progress`:
   1. Read the owner and the number of the project from the link in the [Issue workflow](../../docs/conventions/github-issues.md#workflow). Run `gh project view <project-number> --owner <owner> --format json` to get the project ID.
   2. Run `gh project item-list <project-number> --owner <owner> --format json --limit 1000` to get the Project item ID of the Issue. If the Issue is absent, stop.
   3. Run `gh project field-list <project-number> --owner <owner> --format json` to get the ID of the `Status` field and of its `In Progress` option.
   4. Run `gh project item-edit --id <project-item-id> --project-id <project-id> --field-id <status-field-id> --single-select-option-id <in-progress-option-id>`.
4. Read the Issue, the specification, and the code, patterns, and types that the task affects.
5. Build the task in slices with the skills. For each slice, write a failing test, make it pass, run the focused tests, and create a checkpoint commit with a `Refs` footer, as the [commit message conventions](../../docs/conventions/commit-messages.md) state.
6. Run each check in the `Verification` list of the Issue, except the CI review.
7. If the Issue is the last open Issue of its phase, check the checkpoint of the phase in the plan, as the [task list rules](../../docs/conventions/module-plans.md#task-list) state.
8. If the Issue is the final Prove task, follow the [final Prove task rules](../../docs/conventions/github-issues.md#final-prove-task) for the statuses of the specification and the plan.

## Open the PR

1. Inspect the complete diff, push the branch, and open the PR, as the [pull request conventions](../../docs/conventions/pull-requests.md) and the [label conventions](../../docs/conventions/github-labels.md) state. For the final Prove task, also follow the [final Prove task PR rules](../../docs/conventions/pull-requests.md#final-prove-task-pr).
2. Wait for CI, and fix each failure that the pull request conventions require you to fix.
3. Check the passed items of the Issue, and leave each gap unchecked, as the Issue workflow states:
   1. Immediately before the edit, run `gh issue view <issue-number> --json body --jq .body`, and copy the body to a temporary file.
   2. In `Acceptance criteria` and `Verification`, change each passed item from `[ ]` to `[x]`. Keep all other content.
   3. Run `gh issue edit <issue-number> --body-file <temporary-file>`.
4. Give the squash message in the chat, and tell the maintainer that the PR is ready, or why it cannot become ready. Stop.

Keep the Issue open with the `In Progress` status. The PR closes it after the maintainer merges it.
