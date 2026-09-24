# Branch name conventions

## Overview

A branch name identifies one reviewable change outside `main`.

## When to Follow

Follow this convention when you create a short-lived branch for a pull request.

## Template

```text
<type>/<short-description>
```

| Part | How to write it |
| --- | --- |
| Type | Select a [commit type](commit-messages.md#types). |
| Short description | Use lowercase words separated by hyphens. Do not add an agent or author prefix. |

## Examples

```text
feat/workspace-invitations
fix/duplicate-notifications
docs/git-workflow
ci/pr-title-validation
```
