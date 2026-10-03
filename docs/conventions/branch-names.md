# Branch name conventions

This convention defines the name of a short-lived branch that holds one reviewable change outside `main`. The [agent instructions](../../AGENTS.md) define when to create a branch and how it reaches `main` through a pull request (PR).

## Template

A branch name has a type and a short description.

```text
<type of the change>/<short description of the change>
```

### Type

- Use the [commit type](commit-messages.md#types) of the PR title, such as `docs` for `docs(agents): ...`.
- Do not add the scope of the PR title.

### Short description

- Write lowercase words separated by hyphens.
- Name the change with a noun phrase or a verb phrase, such as `identity-github-login` or `clarify-hexagonal-rules`.
- Do not add another slash.

## Rules

- Do not add an agent or author prefix, such as `claude/` or `codex/`.
- Apply a change of this convention to new branches only. Do not rename a branch that has an open PR.

## Differences from the git-workflow-and-versioning skill

The [`git-workflow-and-versioning` skill](../../.agents/skills/git-workflow-and-versioning/SKILL.md) gives branch prefixes for common kinds of change. This convention applies where the two differ:

- Use the commit type `feat` for a new feature, as the [type rules](#type) state. The skill uses `feature/`.

## Examples

These branch names follow the convention:

| PR title | Branch name |
| --- | --- |
| `feat(identity): log in with GitHub identity and email proof` | `feat/identity-github-login` |
| `test(identity): prove password recovery for provider accounts` | `test/identity-provider-account-recovery` |
| `docs(agents): remove guesswork from hexagonal component rules` | `docs/clarify-hexagonal-rules` |

This branch name does not follow the convention, because `feature` is not a commit type:

| PR title | Branch name |
| --- | --- |
| `feat(identity): log in with GitHub identity and email proof` | `feature/identity-github-login` |
