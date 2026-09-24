# Module plan conventions

Save one plan for each approved specification as `tasks/<module-id>.md`. Use the same module ID as the specification in `docs/specs/`. Track implementation tasks in GitHub Issues and their status in the repository GitHub Project.

Start the plan with its name, module ID, and current status:

```text
# Implementation plan: <capability name>

Module id: `<module-id>`

Status: Draft.
```

Use `Draft` before approval, `Approved` after approval, and `Complete` after final verification. Use these sections in order, as shown in the [Identity plan](../../tasks/identity-signup-and-email-verification.md) and [Workspace plan](../../tasks/workspace-create-read.md):

- `Overview` states the capability, links the approved specification, and names the GitHub Project that tracks tasks.
- `Architecture decisions` records the decisions that set task boundaries. Treat open proposals as undecided.
- `Dependency graph` shows the order between major pieces of work in a Mermaid diagram.
- `Task list` groups numbered tasks into phases. After each phase, add a checkpoint with outcomes that can be checked.
- `Risks and controls` names each material risk, its impact, and its control in a table.

Use `tasks/.todo.md` only while preparing GitHub Issues. Write each task draft with the [Issue fields](github-issues.md#body-fields). After the Issues are in the GitHub Project with `Todo` status, replace the plan's task drafts with an ordered index of Issue links. Do not keep a duplicate task checklist in the plan. Delete `tasks/.todo.md` after every task appears in the Project.

When the module is complete, update its status and record the PR that holds the final verification. Keep the plan's task links and completed checkpoints as the completion record.
