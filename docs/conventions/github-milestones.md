# GitHub milestone conventions

This convention defines the fields and life cycle of the GitHub milestone for 1 approved module plan.

## Template

A milestone has a title, a due date, and a description, which are fields of the milestone form on GitHub.

```text
Plan: https://github.com/vasapolrittideah/flowspace-api/blob/main/tasks/<module ID of the plan>.md
```

### Title

- Write the title in the `title` field of the milestone.
- Copy the capability name from the `# Implementation plan: <capability name>` heading of the plan, without the fixed prefix.
- If the capability name of the plan changes, update the title.

### Due date

- Leave the `due date` field of the milestone empty, because a module plan has no due date.

### Description

- Write the description in the `description` field, with only the line in the code block.
- Link to the plan file on `main`. If the path of the plan file changes, update the link.

## Rules

### Workflow

- When you create the Issues of an approved plan, create 1 milestone for the plan. If the plan already has a milestone, such as when a reapproved plan adds tasks, reuse it.

### Links and tracking

- Assign the milestone to every Issue in the numbered [Task list](module-plans.md#task-list) of the plan. An Issue that the plan mentions only as a dependency keeps the milestone of its own plan.
- Do not use the same milestone for another plan.
- Do not assign the milestone to a PR or to an Issue outside the [Task list](module-plans.md#task-list), such as a follow-up Issue.

### Changes

- If you reuse a closed milestone, reopen it.
- If the [Task list](module-plans.md#task-list) of the plan changes, update the Issue assignments to match.
- After the PR of the final Prove task merges, close the milestone when the plan is `Complete` and every Issue in the milestone is closed.
- Apply a change of this convention only to new milestones and to milestones that a later PR changes. Do not change the assignments of closed Issues or merged PRs.
