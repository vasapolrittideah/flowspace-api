---
name: flowspace-build
description: Implement the next planned task, or every task in an approved plan with auto mode.
---

# FlowSpace Build

Use the `incremental-implementation` and `test-driven-development` skills.

## Modes

Use the text after `$flowspace-build` as the mode and module id. Treat `auto` or `all` as autonomous mode. Without either word, implement the next pending task and stop.

Read the module spec at `docs/specs/<module-id>.md` and the plan at `tasks/<module-id>/plan.md`. Use `tasks/<module-id>/todo.md` to track progress. If the module id is unclear, ask which module to build.

## One task

1. Read the next task and its acceptance criteria.
2. Read the existing code, contracts, and repository instructions that apply.
3. For executable behavior, write a failing test that proves the expected result.
4. Implement the smallest change that passes the test.
5. Run the relevant repository tests and build commands.
6. Mark the task complete after its acceptance criteria pass.
7. Commit only the task files and its status update, then stop.

For documentation or configuration work, use the relevant lint and configuration checks instead of application tests.

## Autonomous mode

1. Require the module spec before implementation. If it is missing, ask the user to run `$flowspace-spec` first.
2. Run `git status --porcelain`. Preserve unrelated local work and exclude it from all task commits.
3. If the plan is missing, use `$flowspace-plan` for the same module id.
4. Present the full plan for approval. If the user already approved that plan, continue within that scope.
5. If you created planning artifacts, commit them before implementation.
6. Execute pending tasks in dependency order with the one-task loop. Continue after each task passes and gets its own commit.
7. If a test or build fails, use the `debugging-and-error-recovery` skill. Stop if you cannot resolve the failure.
8. If the spec leaves a required decision open, ask the user before work that depends on it.
9. Before a destructive or irreversible action, make sure that the user authorized that action.

After a blocker is resolved, `$flowspace-build auto <module-id>` resumes from the next pending task. Report completed tasks, tests, commits, and remaining blockers.
