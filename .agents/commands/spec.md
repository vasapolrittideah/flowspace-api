---
description: Start spec-driven development — write a structured specification before writing code
---

Invoke the agent-skills:spec-driven-development skill.

Begin by understanding what the user wants to build. Ask clarifying questions about:
1. The objective and target users
2. Core features and acceptance criteria
3. Tech stack preferences and constraints
4. Known boundaries (what to always do, ask first about, and never do)

Then generate a structured spec covering all six core areas: objective, commands, project structure, code style, testing strategy, and boundaries.

If the request bundles several independently testable capabilities, first propose a capability map (module ids, dependency direction, build order) per the skill's Phase 0 and get it approved, then spec each module in dependency order.

Save each spec as `docs/specs/<module-id>.md`. Use this path instead of the project-root path suggested by the skill. Run `ls docs/specs/` to list the specs.

If a request includes several capabilities, save its capability map as `docs/specs/maps/<map-id>.md`. Use one map per request and name it after the capabilities it covers. Give each request a distinct map id to avoid overwriting an existing map.

Confirm with the user before proceeding.
