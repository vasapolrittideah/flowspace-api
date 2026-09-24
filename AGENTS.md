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

## Conventions

Read the matching convention before writing or updating an artifact. If a skill gives a different template or file path, use the project convention.

| Work | Convention |
| --- | --- |
| Markdown, English prose, code comments, and agent replies | [Markdown and English prose](docs/conventions/markdown.md) |
| Branch names | [Branches](docs/conventions/branches.md) |
| Checkpoint and squash commit messages | [Commit messages](docs/conventions/commit-messages.md) |
| Module specifications | [Module specs](docs/conventions/module-specs.md) |
| Module plans and task tracking | [Module plans](docs/conventions/module-plans.md) |
| GitHub Issue bodies and task drafts in `tasks/.todo.md` | [GitHub Issues](docs/conventions/github-issues.md) |
| Issue and pull request labels | [GitHub labels](docs/conventions/github-labels.md) |
| Pull requests and suggested squash messages | [Pull requests](docs/conventions/pull-requests.md) |
