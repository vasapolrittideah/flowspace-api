---
name: flowspace-plan
description: Plan one FlowSpace module with tasks, acceptance criteria, dependencies, and verification steps.
---

# FlowSpace Plan

Use the `planning-and-task-breakdown` skill.

Read the requested spec at `docs/specs/<module-id>.md`. If a capability map includes the module, also read `docs/specs/maps/<map-id>.md`. Read the code and contracts that the module reaches.

Plan one module id at a time. For a capability map, follow its build order.

1. Analyze the work without changing application code.
2. Identify dependencies between tasks and components.
3. Slice work into tasks that each complete one path through the system.
4. Give each task acceptance criteria and relevant verification commands.
5. Add checkpoints between phases.
6. Present the plan for user review.

Save the plan as `tasks/<module-id>/plan.md` and its task list as `tasks/<module-id>/todo.md`. Use the same module id as the spec. Do not share these paths between modules.

If the module already has unchecked tasks, ask before replacing its plan.
