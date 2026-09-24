# Repository instructions

Use these rules before each task:

- Before writing code, read [`CONSTRAINTS.md`](CONSTRAINTS.md). Do not weaken these constraints to make a change pass.

For product, architecture, or implementation work, also read:

- [`docs/architecture.md`](docs/architecture.md) for current scope and system behavior.
- [`docs/adr/README.md`](docs/adr/README.md) and relevant architecture decision records (ADRs) for accepted decisions.
- [`docs/technology-stack.md`](docs/technology-stack.md) for selected implementation tools.

Treat open proposals in the architecture document as undecided. Do not use a proposal as an implementation decision without explicit approval.

## Delivery authority

A pull request (PR) proposes changes for review. A squash merge combines all commits in a PR into one commit.

- Prepare and test changes through PRs.
- Do not push directly to `main`, merge a PR, or enable auto-merge.
- The maintainer reviews and squash merges each PR.

## Git workflow

The working tree contains local repository files and changes. A branch holds changes outside `main`. A checkpoint commit records one tested work step. A diff shows the changes between two versions.

1. Inspect the working tree and read the relevant project documents.
2. Preserve work that is outside the task.
3. Create a short-lived branch from the current `main`.
4. Keep `main` ready for deployment.
5. Make small changes and test each change.
6. Create checkpoint commits for the tested changes.
7. Keep unrelated refactoring and formatting separate from behavior changes.
8. Complete the [verification requirements](docs/conventions/pull-requests.md#verification).
9. Open a PR to `main`.
10. Address review comments and rerun the affected commands.
11. After the maintainer merges the PR, remove the branch if it contains no work to preserve.

Keep each branch and PR limited to one reviewable change. Split unrelated changes into separate PRs.

A worktree is a separate checkout of the repository. If tasks run at the same time, use separate worktrees.

## Markdown files

- Hard wrapping inserts manual line breaks within paragraphs or list items. Do not hard-wrap Markdown files. Keep each paragraph and list item on one physical line, regardless of length.
- Give each bullet one main point. Keep its conditions, explanations, and exceptions in the same bullet. Do not split a bullet just because it has multiple sentences.
- Use separate bullets for rules that readers can follow independently.

## Branch names

- Follow the [branch name conventions](docs/conventions/branches.md) when creating a branch.

## Commit messages

- Follow the [commit message conventions](docs/conventions/commit-messages.md) for checkpoint and squash commits.

## Planning and task tracking

- Follow the [module plan conventions](docs/conventions/module-plans.md) for plans and task tracking.

### GitHub Issue descriptions

- Follow the [GitHub Issue conventions](docs/conventions/github-issues.md) when writing Issue bodies or task drafts in `tasks/.todo.md`.

## Issue and pull request labels

- Follow the [GitHub label conventions](docs/conventions/github-labels.md) for Issues and pull requests.

## Pull requests

- Follow the [pull request conventions](docs/conventions/pull-requests.md) when preparing a PR and its suggested squash message.

## English prose

Use these rules for all English prose:

- Before writing any English prose, read the [`simple-english` skill](.agents/skills/simple-english/SKILL.md) and the [`humanizer` skill](.agents/skills/humanizer/SKILL.md). First, use Simple English to make the text easy to understand with common words and short sentences. Then, use Humanizer to make the text read naturally while keeping the Simple English rules.
- Apply both skills without exception to Markdown, commit messages, PR titles and descriptions, code comments, and agent replies.
- Preserve code, identifiers, and tool directives as the skills require. For Go doc comments (comments that document packages or symbols), keep the required symbol prefix and comment syntax.
