# GitHub milestone conventions

This convention defines the GitHub milestone of one approved module plan. A milestone groups the Issues of the plan on GitHub. The [module plan convention](module-plans.md) defines a module plan and its Task list.

## Template

A milestone has a title and a description, which are fields of the milestone form on GitHub.

```text
Plan: https://github.com/vasapolrittideah/flowspace-api/blob/main/tasks/<module ID of the plan>.md
```

### Title

- Write the title in the title field of the milestone.
- Copy the capability name from the `# Implementation plan: <capability name>` heading of the plan, without the fixed prefix.
- If the capability name of the plan changes, update the title.

### Description

- Write the description in the description field, with only the line in the code block.
- Link to the plan file on `main`. If the path of the plan file changes, update the link.

## Rules

### Sections and wording

- Leave the due date field of the milestone empty, because a module plan has no due date.

### Workflow

- When you create the Issues of an approved plan, create one milestone for the plan. If the plan already has a milestone, such as when a reapproved plan adds tasks, reuse it.
- After the PR of the final Prove task merges, close the milestone when the plan is `Complete` and every Issue in the milestone is closed.

### Links and records

- Assign the milestone to every Issue in the numbered Task list of the plan. An Issue that the plan mentions only as a dependency keeps the milestone of its own plan.
- If the Task list of the plan changes, update the Issue assignments to match.
- Do not use the same milestone for another plan.
- Do not assign the milestone to a PR or to an Issue outside the Task list, such as a follow-up Issue.

### Changes

- Apply a change of this convention to new milestones, and to open milestones when you edit them. Do not change the assignments of closed Issues or merged PRs.
