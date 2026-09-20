---
description: Break work into small verifiable tasks and track each task in GitHub
---

Invoke the agent-skills:planning-and-task-breakdown skill.

Read the capability spec at `docs/specs/<module-id>.md`. If a capability map includes the module, read `docs/specs/maps/<map-id>.md` too. Read the codebase sections that the capability affects. Plan one module at a time. Plan modules in the build order from the capability map.

Use GitHub Issues as the task list. Use GitHub Projects to show task status.

1. Enter plan mode. Read files, but do not change implementation code.
2. Identify the dependency graph between components.
3. Slice work vertically. Each task must deliver one complete path.
4. Write acceptance criteria and verification steps for each task in `tasks/.todo.md`.
5. Add checkpoints between phases in the plan document.
6. Save the plan as `tasks/<module-id>.md`.
7. Present the plan and `tasks/.todo.md` for human review.
8. After approval, inspect open issues and projects to avoid duplicate tasks.
9. Run `gh auth status`. If authentication fails, stop and ask the maintainer to run `gh auth login -h github.com -p https -w`.
10. Make sure that GitHub CLI has the `project` scope. If it does not, ask the maintainer to run `gh auth refresh -h github.com -s project`.
11. Reuse the open GitHub Project for the repository. If none exists, create one with the repository name.
12. Create one GitHub Issue from each task in `tasks/.todo.md`. Put all task details in the issue body.
13. Add each issue to the GitHub Project with the `Todo` status.
14. Record each dependency as `Blocked by #<issue-number>` in the dependent issue.
15. Replace the plan Task List with an ordered index of issue links without duplicate checklists.
16. Make sure that every task exists in the project. Delete `tasks/.todo.md` after this succeeds.

If `tasks/.todo.md` exists for the same module, update it in place. If it belongs to another module, stop and ask before changing it. If another incomplete plan or issue set exists for the same module, stop and ask before changing it.

Keep each implementation commit limited to one issue. Add `Refs: #<issue-number>` to every commit message. Add `Closes #<issue-number>` to the pull request that completes the task.
