---
description: Implement GitHub Issues with tests, commits, and verification
---

Invoke the agent-skills:incremental-implementation skill and the agent-skills:test-driven-development skill.

## Select the module

Use the first `$ARGUMENTS` value that is not `auto` or `all` as the module id. Read its plan from `tasks/<module-id>.md`. If no module id is present, use the only plan in `tasks/`. If `tasks/` contains multiple plans, stop and ask for the module id.

Before you read issues, run `gh auth status --active --hostname github.com` and `gh api user --jq .login`. If a command reports a connection or DNS error, retry it with network access outside the sandbox. Keep the current credentials during this retry. If GitHub rejects the credentials after a successful connection, ask the maintainer to run `gh auth login -h github.com -p https -w` and stop.

Make sure that the active token has the `project` scope. If this scope is absent, ask the maintainer to run `gh auth refresh -h github.com -s project` and stop. Repeat the authentication checks after the command succeeds. If `tasks/.todo.md` exists, stop because task migration is incomplete.

Use the issue links in the plan as the task list. Read acceptance criteria, verification steps, and dependencies from each issue.

## Modes

- `/build <module-id>` implements the next ready open issue and then stops.
- `/build <module-id> auto` implements every issue in dependency order after one approval.
- `/build <module-id> all` is the same as `auto`.

An issue is ready when each issue in its `Blocked by` line is complete. In autonomous mode, a verified commit completes the dependency for the current run.

## Implement one issue

1. Select the first ready open issue from the plan.
2. Set its GitHub Project status to `In Progress`:
   - Run `gh repo view --json nameWithOwner,owner` to get the repository owner and name.
   - Run `gh project list --owner <owner> --format json` and select the open Project for the repository. If the result is unclear, stop.
   - Run `gh project item-list <project-number> --owner <owner> --format json --limit 1000` to get the issue's Project item ID. If the issue is absent, stop.
   - Run `gh project field-list <project-number> --owner <owner> --format json` to get the `Status` field ID and `In Progress` option ID.
   - Run `gh project item-edit --id <project-item-id> --project-id <project-id> --field-id <status-field-id> --single-select-option-id <in-progress-option-id>`.
3. Read the issue acceptance criteria and likely files.
4. Read the relevant code, patterns, and types.
5. Write a failing test for the expected behavior.
6. Implement the minimum change that passes the test.
7. Run the focused tests, required checks, and build commands from the issue.
8. Run the affected regression tests.
9. Update the issue description after verification:
   - Immediately before the edit, fetch the current body with `gh issue view <issue-number> --json body --jq .body`.
   - Copy the current body to a temporary file. Preserve all content outside `Acceptance criteria` and `Verification`.
   - In those sections, change each completed checkbox from `[ ]` to `[x]`. Leave failed and unrun items unchecked.
   - Run `gh issue edit <issue-number> --body-file <temporary-file>` with the complete updated body.
10. Inspect the staged diff and exclude unrelated changes.
11. Commit only the issue changes. Add `Refs: #<issue-number>` before the co-author trailers.
12. Record the exact verification results for the pull request.

Leave the issue open and keep its status as `In Progress`. The pull request closes it after the maintainer merges the change.

## Autonomous mode

Use autonomous mode only after the human approves the full plan.

1. Make sure that `docs/specs/<module-id>.md` and `tasks/<module-id>.md` exist.
2. Run `git status --porcelain` and preserve unrelated work.
3. Present the issue order and wait for clear approval.
4. Implement each issue with the full issue loop.
5. Create one tested commit per issue with the matching `Refs: #<issue-number>` footer.
6. Stop when an issue fails, needs an undecided product choice, or requires an irreversible action.
7. Prepare the pull request with `Closes #<issue-number>` for every completed issue.
8. Summarize completed issues, tests, commits, and remaining work.

Do not close issues or set their status to `Done` before the maintainer merges the pull request.

If any step fails, follow the agent-skills:debugging-and-error-recovery skill.
