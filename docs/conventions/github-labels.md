# GitHub label conventions

This convention defines which labels to apply to GitHub Issues and pull requests (PRs). [`.github/labels.json`](../../.github/labels.json) lists every label, and the `Sync labels` workflow makes the labels on GitHub match the file after a change to it reaches `main`. A type label is a `type:*` label that names the type of a change. An area label is an `area:*` label that names a repository area. The [commit message convention](commit-messages.md) defines the type, the scope, the main change, and a change that breaks callers.

## Rules

### Links and tracking

- Use only the labels in `.github/labels.json`. To add, change, or remove a label, change the file in a PR.
- Apply exactly one type label, with the name of the type in the PR title, such as `type:docs` for `docs(agents): ...`.
- If a PR title has a scope, apply the area label with the name of that scope, and no other area label.
- If a PR title has no scope, apply the area label of each row in the [scope table](commit-messages.md#scopes) whose `Paths` column covers part of the main change. If no row covers any part of the main change, apply no area label.
- Apply `breaking` when the PR title has `!`.
- Apply `migration` when the work adds or changes a file under `services/*/db/migrations/`.
- For an Issue, apply the labels that the PR of the task will have. Choose them from the main change that the `Description` and `Files likely touched` of the Issue plan.
- If a PR adds a scope to the scope table, add an area label with the same name to `.github/labels.json` in the same PR.

### Changes

- Apply a change of this convention to new Issues and PRs, and to open Issues and PRs when you edit them. Do not change the labels of closed Issues or merged PRs.
