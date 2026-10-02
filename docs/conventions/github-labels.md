# GitHub label conventions

This convention defines which labels to apply to GitHub Issues and pull requests (PRs).

## Rules

- Use only labels from [`.github/labels.json`](../../.github/labels.json).
- Apply exactly one `type:*` label that matches the work.
- If the title has a scope, apply the `area:*` label that has the same name as the [commit scope](commit-messages.md#scopes). If the scope names one shared package under root `internal/`, such as `authn`, use `area:shared`. Add an area label for each other area that the work affects.
- If the title has no scope, you can omit the `area:*` labels.
- Apply `breaking` and `migration` when they match the work.
- Keep the title, description, and labels consistent with the final work.
- Review the labels again when the title, description, or scope changes.
