# GitHub label conventions

This convention defines which labels to apply to GitHub Issues and pull requests (PRs). Follow it when creating or updating an Issue or PR.

## Rules

- Use only labels from [`.github/labels.json`](../../.github/labels.json).
- Apply exactly one `type:*` label that matches the work.
- Apply one or more relevant `area:*` labels. Use multiple area labels when the work affects multiple areas.
- Apply `breaking` and `migration` when they match the work.
- Keep the title, description, and labels consistent with the final work.
- Review the labels again when the title, description, or scope changes.

## Examples

Documentation for agent instructions uses `type:docs` and `area:agents`.

A breaking Workspace change with a database migration that needs controlled deployment uses `type:feat`, `area:workspace`, `breaking`, and `migration`.
