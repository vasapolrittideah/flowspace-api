# GitHub label conventions

This convention defines which labels to apply to GitHub Issues and pull requests (PRs).

[`.github/labels.json`](../../.github/labels.json) lists every label, and the `Sync labels` workflow makes the labels on GitHub match the file after a change to it reaches `main`.

## Rules

### Links and tracking

- Apply exactly 1 type label, with the name of the type in the PR title, such as `type:docs` for `docs(agents): ...`.
- If a PR title has a scope, apply the area label with the name of that scope, and no other area label.
- If a PR title has no scope, apply the area label of each row in the [Scopes](commit-messages.md#scopes) table whose `Paths` column covers part of the main change. If no row covers any part of the main change, apply no area label.
- If the PR title has `!`, apply `breaking`.
- If the work adds or changes a file under `services/*/db/migrations/`, apply `migration`.
- For an Issue, apply the labels that the PR of the task will have. Choose them from the main change that the [Description](github-issues.md#description) and [Files likely touched](github-issues.md#files-likely-touched) of the Issue plan.
- Use only the labels in `.github/labels.json`.
- If a PR adds a scope to the [Scopes](commit-messages.md#scopes) table, add an area label with the same name to `.github/labels.json` in the same PR.

### Changes

- To add, change, or remove a label, change `.github/labels.json` in a PR.
- Apply a change of this convention to new Issues and PRs, and to open Issues and PRs when you edit them. Do not change the labels of closed Issues or merged PRs.
