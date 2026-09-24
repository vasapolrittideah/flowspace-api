# GitHub Issue conventions

## Overview

A GitHub Issue tracks one task from an approved module plan. The [agent instructions](../../AGENTS.md) define the workflow and authority for creating Issues.

## When to Follow

Follow this convention when writing a task draft in `tasks/.todo.md` or creating or updating an Issue. Before creating the Issue, compare its title, body, acceptance criteria, verification, dependencies, and file list with the approved specification and module plan. After creating it, apply the [Issue labels](github-labels.md), add it to the repository GitHub Project with `Todo` status, and record its numbered dependencies.

## Template

Use one Issue for each task. Put the task title in the Issue title. Use the body fields below in this order, with the same spelling and capitalization.

### Title

State the task outcome. Keep the title and body consistent with the approved specification, module plan, and task scope.

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

#### Module

Use the module ID from the approved specification.

#### Description

State the task outcome and scope in one paragraph.

#### Acceptance criteria

Give each independently checkable outcome one `- [ ]` item. Use as many items as the task needs.

#### Verification

Give each command one `- [ ]` item and name the behavior it checks. Keep inspection of a command's output in the same item when they form one check. Put separate manual checks in separate items.

#### Dependencies

Write `None.` when no Issue blocks the task. Otherwise, write one `Blocked by` sentence with the blocking Issue numbers. For two blockers, use `Blocked by #45 and #46.` For three or more, separate numbers with commas and put `and` before the last number. Add a prerequisite without an Issue number as a separate sentence. Do not use semicolons or repeat `Blocked by` in the same sentence.

#### Files likely touched

List source, test, contract, and configuration files that the task is likely to create or change. For generated output, list only its folder with a trailing slash and `(generated output)`, such as `gen/go/flowspace/identity/v1/` (generated output). Do not list generated file names.

#### Estimated scope

State the expected size of the task based on the work described above.

## Examples

The [Workspace creation Issue](https://github.com/vasapolrittideah/flowspace-api/issues/48) shows a complete Issue using this template.

Use these exact dependency forms for zero, one, two, or three blockers:

```text
Dependencies: None.
Dependencies: Blocked by #45.
Dependencies: Blocked by #45 and #46.
Dependencies: Blocked by #45, #46, and #47.
```
