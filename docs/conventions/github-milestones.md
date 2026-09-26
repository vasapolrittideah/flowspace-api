# GitHub milestone conventions

This convention links one repository milestone to one approved module plan.

## Template

| Part | Content and format |
| --- | --- |
| Title | The capability name in the plan's `# Implementation plan: <capability name>` heading, without the fixed prefix. |
| Description | `Plan: https://github.com/vasapolrittideah/flowspace-api/blob/main/tasks/<module-id>.md` |
| Due date | The plan's due date, if it has one. |

## Rules

- Create or reuse one milestone for each approved plan. Do not use the same milestone for another plan.
- Keep the title identical to the capability name in the plan heading. Keep the description to the plan link in the template, so the plan remains the source for scope and checkpoints.
- Assign the milestone to every Issue in the plan's numbered Task list. An Issue mentioned only as a dependency keeps the milestone of its own plan.
- Do not assign the milestone to linked pull requests.
- If the plan name, file path, or Task list changes, update the milestone title, description, or Issue assignments to match.
