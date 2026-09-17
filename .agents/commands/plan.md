---
description: Break work into small verifiable tasks with acceptance criteria and dependency ordering
---

Invoke the agent-skills:planning-and-task-breakdown skill.

Read the capability's spec at `docs/specs/<module-id>.md` — and the `docs/specs/maps/<map-id>.md` that bundles it, when it is one module of several — then the codebase sections it reaches. Plan one module id at a time; a capability map is planned in its build order, not all at once. Then:

1. Enter plan mode — read only, no code changes
2. Identify the dependency graph between components
3. Slice work vertically (one complete path per task, not horizontal layers)
4. Write tasks with acceptance criteria and verification steps
5. Add checkpoints between phases
6. Present the plan for human review

Save each plan as `tasks/<module-id>/plan.md` and its task list as `tasks/<module-id>/todo.md`, keyed by the same module id its spec carries in `docs/specs/` — `ls tasks/` is the index. Two capabilities planned in parallel never share a path, so nothing is overwritten by being next.

Re-planning a module id that already holds unchecked tasks is the one collision left: stop and ask before writing.
