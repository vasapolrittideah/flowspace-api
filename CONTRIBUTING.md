# Contributing to FlowSpace

Use this workflow to guide the agent from an idea to a reviewed change.

## Development workflow

Use the full command workflow for a new product capability:

1. Run `/spec <request>` to define the behavior and acceptance criteria. Review and approve the specification before planning.
2. Run `/plan <module-id>` to create the module plan and proposed GitHub Issues. Review and approve them before issue creation.
3. Run `/build <module-id>` to implement the next ready issue. Use `/build <module-id> auto` only after you approve the complete plan.
4. Run `/review` to review correctness, readability, architecture, security, and performance. Resolve important findings before the final checks.
5. Run `/ship` to run the launch checks and produce a go or no-go decision. Resolve all launch blockers before handoff.

The `/build` command uses test-driven development and runs the checks from each issue. Use `/test` for a focused change outside the module workflow.

Each command stores durable results in the repository or GitHub. Specifications live in `docs/specs/`, module plans live in `tasks/`, and task status lives in GitHub Projects.

For a small documentation or maintenance change, ask the agent to implement it directly. The change does not need a product specification or module plan.
