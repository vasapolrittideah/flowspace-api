# Module plan conventions

## File location

Save one plan for each approved specification as `tasks/<module-id>.md`. Use the same module ID as the specification in `docs/specs/`.

## Header

Start the plan with its name, module ID, and current status:

```text
# Implementation plan: <capability name>

Module id: `<module-id>`

Status: Draft.
```

## Status

Use `Draft` before approval, `Approved` after approval, and `Complete` after final verification.

## Sections

Use these sections in order, as shown in the [Identity plan](../../tasks/identity-signup-and-email-verification.md) and [Workspace plan](../../tasks/workspace-create-read.md):

| Section | Content |
| --- | --- |
| `Overview` | Capability, link to the approved specification, and GitHub Project that tracks tasks |
| `Architecture decisions` | Decisions that set task boundaries. Treat open proposals as undecided. |
| `Dependency graph` | Order between major pieces of work in a Mermaid diagram |
| `Task list` | Numbered tasks grouped into phases, with a checkpoint of outcomes that can be checked after each phase |
| `Risks and controls` | Each material risk, its impact, and its control in a table |

## Task tracking

Track implementation tasks in GitHub Issues and their status in the repository GitHub Project.

Use `tasks/.todo.md` only while preparing GitHub Issues. Write each task draft with the [Issue fields](github-issues.md#body-fields). After the Issues are in the GitHub Project with `Todo` status, replace the plan's task drafts with an ordered index of Issue links. Do not keep a duplicate task checklist in the plan. Delete `tasks/.todo.md` after every task appears in the Project.

## Completion

When the module is complete, update its status and record the PR that holds the final verification. Keep the plan's task links and completed checkpoints as the completion record.
