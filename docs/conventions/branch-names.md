# Branch name conventions

This convention defines the name of a short-lived branch that holds 1 reviewable change outside `main`.

The [Repository instructions](../../AGENTS.md) define when to create a branch and how it reaches `main` through a pull request (PR).

## Template

A branch name has a type and a short description.

```text
<type of the change>/<short description of the change>
```

### Type

- Do not add the scope of the PR title.
- Use the commit type of the PR title, from [Types](commit-messages.md#types), such as `docs` for `docs(agents): ...`.

### Short description

- Write lowercase words separated by hyphens.
- Do not add another slash.
- Name the change with a noun phrase or a verb phrase, such as `identity-github-login` or `clarify-hexagonal-rules`.

## Rules

- Do not add an agent or author prefix, such as `claude/` or `codex/`.
- Apply a change of this convention to new branches and to existing branches that a later PR renames. Do not rename a branch that has an open PR.

## Differences from the git-workflow-and-versioning skill

This convention applies where its branch naming rules differ from the [`git-workflow-and-versioning` skill](../../.agents/skills/git-workflow-and-versioning/SKILL.md):

- Follow the [Type](#type) rules for the branch prefix. The skill uses `feature/` for a new feature.

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
