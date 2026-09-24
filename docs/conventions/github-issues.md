# GitHub Issue conventions

This convention defines the title and body of a GitHub Issue for one task. Follow it when drafting an Issue in `tasks/.todo.md` or creating or updating an Issue.

## Rules

- Follow the workflow and authority in the [agent instructions](../../AGENTS.md) when creating Issues.
- Before creating an Issue, compare its title, body, acceptance criteria, verification, dependencies, and file list with the approved specification and module plan.
- After creating an Issue, apply the [Issue labels](github-labels.md), add it to the repository GitHub Project with `Todo` status, and record its numbered dependencies.

## Template

Use one Issue for each task. Use the body fields below in this order, with the same spelling and capitalization.

State the task outcome in the Issue title. Keep the title and body consistent with the approved specification, module plan, and task scope.

### Body fields

```text
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

| Field | How to write it |
| --- | --- |
| Module | Use the module ID from the approved specification. |
| Description | State the task outcome and scope in one paragraph. |
| Acceptance criteria | Give each independently checkable outcome one `- [ ]` item. Use as many items as the task needs. |
| Verification | Give each command one `- [ ]` item and name the behavior it checks. Keep inspection of a command's output in the same item when they form one check. Put separate manual checks in separate items. |
| Dependencies | Write `None.` when no Issue blocks the task. Otherwise, write one `Blocked by` sentence with the blocking Issue numbers. Separate three or more numbers with commas and put `and` before the last number. Add a prerequisite without an Issue number as a separate sentence. Do not use semicolons or repeat `Blocked by` in the same sentence. |
| Files likely touched | List source, test, contract, and configuration files that the task is likely to create or change. For generated output, list only its folder with a trailing slash and `(generated output)`, such as `gen/go/flowspace/identity/v1/` (generated output). Do not list generated file names. |
| Estimated scope | State the expected size of the task based on the work described above. |

## Examples

The [Workspace creation Issue](https://github.com/vasapolrittideah/flowspace-api/issues/48) shows a complete Issue using this template.

Use these exact dependency forms for zero, one, two, or three blockers:

```text
Dependencies: None.
Dependencies: Blocked by #45.
Dependencies: Blocked by #45 and #46.
Dependencies: Blocked by #45, #46, and #47.
```
