# Contributing to FlowSpace

Use the repository commands to move work from a request to a reviewed pull request. Choose the shortest workflow that covers the change.

## Command workflow

For a new capability, follow `/spec <request>` → `/plan <module-id>` → `/build <module-id>` → `/review` → `/ship`.

For a focused feature or bug fix that needs no module plan, follow `/test <request>` → `/review` → `/ship`. Use `/constraints` when you need to set or inspect the quality rules.

| Command | Use it when | Result |
| --- | --- | --- |
| `/spec <request>` | You need to define a new capability. | Writes a specification with scope and acceptance criteria in `docs/specs/`. Approve it before `/plan`. |
| `/plan <module-id>` | The specification is approved. | Writes `tasks/<module-id>.md`. After you approve the plan, it creates linked GitHub Issues in dependency order. |
| `/build <module-id>` | You want to implement the next ready issue. | Adds a failing test, implements the issue, runs checks, and makes one tested commit. Run it again for the next issue after the previous pull request merges. |
| `/build <module-id> auto` | You approved the full plan and want to implement all its issues in order. | Runs the same steps for each issue. `all` is an alias for `auto`. |
| `/test <request>` | A focused feature or bug fix needs no module plan. | Writes a failing test, implements the change, and runs regression tests. |
| `/review` | The change is ready for review. | Reports findings on correctness, readability, architecture, security, and performance. Resolve Critical and Important findings and rerun affected checks. |
| `/ship` | Review findings are resolved. | Returns a go or no-go decision with blockers, risks, and a rollback plan for maintainer review. |
| `/constraints [check\|guard\|ratchet]` | You need to set or inspect the quality rules. | With no argument, sets up the rules. `check` runs them. `guard` finds weaker rules. `ratchet` updates measured limits. |

If `tasks/` contains only one plan, you can omit `<module-id>` from `/build`.

## Pull request handoff

Keep one reviewable change on each branch and pull request. Run the required checks from `Taskfile.yaml`, inspect the complete diff, and record the exact results in the pull request template. The maintainer reviews and squash merges the pull request.

## Agents without command support

Some agents do not register repository commands as slash commands. Tell the agent to read the matching command file and give it the arguments from the command table. Write the rest of the prompt for your task.

| Command | Command file | Arguments |
| --- | --- | --- |
| `/spec` | `@.agents/commands/spec.md` | `<request>` |
| `/plan` | `@.agents/commands/plan.md` | `<module-id>` |
| `/build` | `@.agents/commands/build.md` | `[<module-id>] [auto\|all]` |
| `/test` | `@.agents/commands/test.md` | `<request>` |
| `/review` | `@.agents/commands/review.md` | None |
| `/ship` | `@.agents/commands/ship.md` | None |
| `/constraints` | `@.agents/commands/constraints.md` | `[check\|guard\|ratchet]` |
