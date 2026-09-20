# Contributing to FlowSpace

Use this workflow to guide the agent from an idea to a reviewed change.

## Choose a workflow

Choose the workflow that matches the change:

- For a new product capability, use the full development workflow.
- For a focused change outside the module workflow, use `/test <request>`.
- For a small documentation or maintenance change, ask the agent to implement it directly. The change does not need a specification or module plan.

## Full development workflow

Use the full command workflow for a new product capability:

1. Run `/spec <request>` to define the behavior and acceptance criteria. Review and approve the specification before planning.
2. Run `/plan <module-id>` to create the module plan and proposed GitHub Issues. Review and approve them before issue creation.
3. Run `/build <module-id>` to implement the next ready issue. Use `/build <module-id> auto` only after you approve the complete plan.
4. Run `/review` to review correctness, readability, architecture, security, and performance. Resolve important findings before the final checks.
5. Run `/ship` to run the launch checks and produce a go or no-go decision. Resolve all launch blockers before handoff.

The build workflow uses test-driven development and runs the checks from each issue.

Each workflow stores durable results in the repository or GitHub. Specifications live in `docs/specs/`, module plans live in `tasks/`, and task status lives in GitHub Projects.

## Run workflows in Codex

Codex does not expose the files in `.agents/commands/` as slash commands. In Codex, use these prompts:

| Command | Codex prompt |
| --- | --- |
| `/spec <request>` | `Read and follow @.agents/commands/spec.md. Use <request> as the request.` |
| `/plan <module-id>` | `Read and follow @.agents/commands/plan.md. Use <module-id> as the module id.` |
| `/build <module-id>` | `Read and follow @.agents/commands/build.md. Use <module-id> as the module id.` |
| `/build <module-id> auto` | `Read and follow @.agents/commands/build.md. Use <module-id> as the module id. Use auto mode.` |
| `/test <request>` | `Read and follow @.agents/commands/test.md for <request>.` |
| `/review` | `Read and follow @.agents/commands/review.md for the current changes.` |
| `/ship` | `Read and follow @.agents/commands/ship.md for the current changes.` |
| `/constraints [check\|guard\|ratchet]` | `Read and follow @.agents/commands/constraints.md. Use check, guard, or ratchet as the argument when needed.` |

The command file supplies the workflow instructions. If the file reads `$ARGUMENTS`, the prompt supplies those values.
