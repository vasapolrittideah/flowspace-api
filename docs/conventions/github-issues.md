# GitHub Issue conventions

This convention defines the title and body of a GitHub Issue for one task.

## Template

An Issue has a title and the body fields shown below.

```markdown
Module: `<module-id>`

Description: <task outcome and scope>

Acceptance criteria:

- [ ] <outcome that can be checked on its own>

Verification:

- [ ] Run `<command>` to check <behavior>.

Dependencies: None.

Files likely touched:

- `<source, test, contract, or configuration path>`

Estimated scope: <expected size>.
```

| Field | Content and format |
| --- | --- |
| Title | The task outcome. |
| Module | Use the module ID from the approved specification. |
| Description | State the task outcome and scope in one paragraph. |
| Acceptance criteria | Independently checkable outcomes, each as a `- [ ]` item. |
| Verification | Commands and the behavior each command checks, each as a `- [ ]` item. |
| Dependencies | Blocking Issue numbers or `None.` when there are no blockers. Other prerequisites follow in a separate sentence. |
| Files likely touched | Source, test, contract, and configuration paths. Generated output appears as a folder with a trailing slash and `(generated output)`, such as `gen/go/flowspace/identity/v1/` (generated output). |
| Estimated scope | State the expected size of the task based on the work described above. |

## Rules

- Follow the workflow and authority in the [agent instructions](../../AGENTS.md) when creating Issues.
- Use one Issue for each task. Use the body fields in the template order, with the same spelling and capitalization.
- State the task outcome in the Issue title. Keep the title and body consistent with the approved specification, module plan, and task scope.
- Before creating an Issue, compare its title, body, acceptance criteria, verification, dependencies, and file list with the approved specification and module plan.
- After creating an Issue, apply the [Issue labels](github-labels.md), add it to the repository GitHub Project with `Todo` status, and record its numbered dependencies. When creating it from `tasks/.todo.md`, also assign the milestone for its approved module plan.
- Use as many acceptance criteria items as the task needs.
- Keep inspection of a command's output in the same Verification item when they form one check. Put separate manual checks in separate items.
- Write `Dependencies: None.` when no Issue blocks the task. Otherwise, write one `Blocked by` sentence with the blocking Issue numbers. Separate three or more numbers with commas and put `and` before the last number. Add a prerequisite without an Issue number as a separate sentence. Do not use semicolons or repeat `Blocked by` in the same sentence.
- Use the exact dependency forms shown in Examples.
- For generated output, list only its folder. Do not list generated file names.

## Examples

The [Workspace creation Issue](https://github.com/vasapolrittideah/flowspace-api/issues/48) shows a complete Issue using this template.

Dependency forms for zero, one, two, or three blockers:

```text
Dependencies: None.
Dependencies: Blocked by #45.
Dependencies: Blocked by #45 and #46.
Dependencies: Blocked by #45, #46, and #47.
```
