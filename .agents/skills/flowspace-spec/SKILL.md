---
name: flowspace-spec
description: Write a FlowSpace module spec or capability map before implementation.
---

# FlowSpace Spec

Use the `spec-driven-development` skill.

Read the repository architecture, accepted decisions, technology stack, and project structure before asking about choices they already define.

Ask about missing requirements for the objective, users, features, acceptance criteria, and scope boundaries. Preserve known requirements and constraints.

Cover these areas in the spec:

1. Objective and expected behavior.
2. Commands and public interfaces.
3. Project structure: List the folders this capability reaches. Use `docs/project-structure.md` for the repository layout.
4. Deliberate choices: Explain decisions that a later reader can otherwise undo. Do not repeat naming or layering rules.
5. Testing strategy and acceptance evidence.
6. Boundaries and decisions that require user input.

If the request contains several independently testable capabilities, propose a capability map with module ids, dependency direction, and build order. Get approval before writing the module specs in dependency order.

Save each spec as `docs/specs/<module-id>.md`. Save capability maps as `docs/specs/maps/<map-id>.md`. Use a distinct id for each module and map. Present the spec for approval before implementation.

After implementation, move lasting rules to `docs/architecture.md` or an ADR. Use the `documentation-and-adrs` skill for that step.
