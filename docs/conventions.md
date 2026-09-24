# Project conventions

Use this file for the format of project artifacts. The [agent instructions](../AGENTS.md) define the workflow and authority for creating them.

## GitHub Issues

Use one Issue for each task in an approved module plan. Put the task title in the Issue title. Keep the Issue body consistent with the approved specification, the module plan, and the task scope.

### Body fields

Use these labels in this order, with the same spelling and capitalization:

```text
Module:
Description:
Acceptance criteria:
Verification:
Dependencies:
Files likely touched:
Estimated scope:
```

- `Module:` contains the module ID from the approved specification.
- `Description:` states the outcome and scope of this task in one paragraph.
- `Acceptance criteria:` contains one `- [ ]` item for each outcome that can be checked on its own. Use as many items as the task needs.
- `Verification:` contains one `- [ ]` item per command. Name the behavior that each command checks. Keep inspection of a command's output in the same item when they form one check. Put separate manual checks in separate items.
- `Dependencies:` names blocking Issue numbers in one sentence. Write `None.` when no Issue blocks the task. Add a prerequisite without an Issue number as a separate sentence.
- `Files likely touched:` lists the source, test, contract, and configuration files that the task is likely to create or change. For generated output, list only the folder with a trailing slash and `(generated output)`, such as `gen/go/flowspace/identity/v1/` (generated output). Do not list generated file names.
- `Estimated scope:` states the expected size of the task based on the work described above.

### Dependencies format

Use these exact forms for zero, one, two, or three blockers:

```text
Dependencies: None.
Dependencies: Blocked by #45.
Dependencies: Blocked by #45 and #46.
Dependencies: Blocked by #45, #46, and #47.
```

For more blockers, continue the comma-separated list and put `and` before the last number. Write `Blocked by` once. Do not separate blockers with semicolons or repeat `Blocked by` in the same sentence.

### Final check

Before creating the Issue, compare its title, body, acceptance criteria, verification, dependencies, and file list with the approved specification and module plan. After creating it, apply the [Issue labels](../AGENTS.md#issue-and-pull-request-labels), add it to the repository GitHub Project with `Todo` status, and record its numbered dependencies.
