---
description: Break work into small verifiable tasks and track each task in GitHub
---

Invoke the agent-skills:planning-and-task-breakdown skill.

Before planning, read the [module plan conventions](../../docs/conventions/module-plans.md), [GitHub Issue conventions](../../docs/conventions/github-issues.md), and [GitHub milestone conventions](../../docs/conventions/github-milestones.md). Use their templates and file paths instead of the generic skill defaults.

Read the capability spec at `docs/specs/<module-id>.md`. If a capability map includes the module, read `docs/specs/maps/<map-id>.md` too. Read the codebase sections that the capability affects. Plan one module at a time. Plan modules in the build order from the capability map.

Use GitHub Issues as the task list. Use GitHub Projects to show task status.

1. Enter plan mode. Read files, but do not change implementation code.
2. Identify the dependency graph between components.
3. Slice work vertically. Each task must deliver one complete path.
4. Write each task in `tasks/.todo.md` using the [GitHub Issue conventions](../../docs/conventions/github-issues.md).
5. Add checkpoints between phases in the plan document.
6. Save the plan as `tasks/<module-id>.md` using the [module plan conventions](../../docs/conventions/module-plans.md).
7. Present the plan and `tasks/.todo.md` for human review.
8. After approval, inspect open issues and projects to avoid duplicate tasks.
9. Run `gh auth status --active --hostname github.com` and `gh api user --jq .login`. If either reports a connection or DNS error, retry both outside the sandbox with the current credentials. If GitHub rejects the credentials after a successful connection, ask the maintainer to run `gh auth login -h github.com -p https -w` and stop.
10. Make sure that the active token has the `project` scope. If it does not, ask the maintainer to run `gh auth refresh -h github.com -s project` and stop.
11. Reuse the open GitHub Project for the repository. If none exists, create one with the repository name.
12. Create or reuse the approved plan's GitHub milestone.
13. Create one GitHub Issue from each task in `tasks/.todo.md` using the Issue and milestone conventions.
14. Add each issue to the GitHub Project with the `Todo` status.
15. Record each dependency as `Blocked by #<issue-number>` in the dependent issue.
16. Replace the plan Task List with an ordered index of issue links without duplicate checklists.
17. Make sure that `gh project item-list` shows every new issue with `Todo` status and `gh issue list --milestone "<milestone title>" --state all --limit 1000` shows every Task Issue in the plan.
18. Delete `tasks/.todo.md` after both checks succeed. An issue-side Project link alone is not enough.

If `tasks/.todo.md` exists for the same module, update it in place. If it belongs to another module, stop and ask before changing it. If another incomplete plan or issue set exists for the same module, stop and ask before changing it.

Keep each implementation commit limited to one issue. Add `Refs: #<issue-number>` to every commit message. Add `Closes #<issue-number>` to the pull request that completes the task.
