---
name: flowspace-constraints
description: Set up, check, guard, or ratchet the quality constraints for this repository.
---

# FlowSpace Constraints

Use the `constraint-driven-development` skill. Read the mode from the text after `$flowspace-constraints`. Without a mode, set up or revise the repository constraints within the requested scope.

## Setup

1. Read `CONSTRAINTS.md`, `go.mod`, `Taskfile.yaml`, existing tests, lint configuration, coverage output, and CI workflows.
2. Report the existing tools and quality gates. Reuse them before adding tools.
3. Ask only about missing choices, one question at a time, with a usable default. Limit the interview to four questions.
4. Cover the required dimensions, block or warn behavior, target values, and local execution budget.
5. Record the floor, enforced numbers, measured values, and exceptions in `CONSTRAINTS.md`. Give each number a reason.
6. If a selected dimension lacks a runnable check, add one to `Taskfile.yaml` with the existing tool conventions.
7. Put quick edit checks in `check:fast` and task checks in `check:task`. Keep the budgets in `CONSTRAINTS.md`.
8. Make sure that `AGENTS.md` directs agents to read and preserve the constraints.
9. Run the selected checks. Fix failures without lowering thresholds or disabling gates.

Do not weaken `CONSTRAINTS.md` to make a change pass. A constraint exception needs the review, owner, and expiry required by that file.

## Modes

- `$flowspace-constraints check`: Run `task check:task` and report failures, warnings, and current measurements.
- `$flowspace-constraints guard`: Inspect the diff for lowered thresholds, skipped tests, removed assertions, suppression comments, unfinished stubs, and new exceptions.
- `$flowspace-constraints ratchet`: Measure the current values and raise the floor where the measurements support it. Preserve stricter existing limits.
