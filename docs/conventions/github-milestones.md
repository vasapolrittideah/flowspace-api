# GitHub milestone conventions

This convention links one repository milestone to one approved module plan.

## Template

A milestone has the following fields:

```text
Title: <capability name of the plan>
Description: Plan: https://github.com/vasapolrittideah/flowspace-api/blob/main/tasks/<module ID of the plan>.md
Due date: <due date of the plan, if any>
```

### Title

- Copy the capability name from the plan's `# Implementation plan: <capability name>` heading, without the fixed prefix.
- If the plan name changes, update the title.

### Description

- Write only the plan link, so the plan remains the source for scope and checkpoints.
- If the plan file path changes, update the link.

### Due date

- Copy the due date from the plan.

## Rules

- Create or reuse one milestone for each approved plan. Do not use the same milestone for another plan.
- Assign the milestone to every Issue in the plan's numbered Task list. An Issue mentioned only as a dependency keeps the milestone of its own plan.
- If the plan's Task list changes, update the Issue assignments to match.
- Do not assign the milestone to linked pull requests.
