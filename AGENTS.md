# Repository instructions

Follow these repository instructions for every task. Before making changes, read [`CONSTRAINTS.md`](CONSTRAINTS.md) and the matching convention in the table below. Do not weaken the constraints to make a change pass. For product, architecture, or implementation work, also read [`docs/architecture.md`](docs/architecture.md), relevant [architecture decision records (ADRs)](docs/adr/README.md), and [`docs/technology-stack.md`](docs/technology-stack.md). Treat open architecture proposals as undecided until explicitly approved.

## Delivery authority

A pull request (PR) proposes changes for review. A squash merge combines all commits in a PR into one commit.

- Prepare and test changes on a branch, then submit them for review through a PR.
- Do not push directly to `main`, merge a PR, or enable auto-merge.
- The maintainer reviews and squash merges each PR.

## Git workflow

The working tree contains local repository files and changes. A branch holds changes outside `main`. A checkpoint commit records one tested work step. A diff shows the changes between two versions.

1. Inspect the working tree and read the relevant project documents.
2. Preserve work that is outside the task.
3. For new work, create a short-lived branch from the current `main`. When updating a PR, continue on its branch.
4. Keep `main` ready for deployment.
5. Make small changes and test each change.
6. Create checkpoint commits for the tested changes.
7. Keep unrelated refactoring and formatting separate from behavior changes.
8. Run the applicable checks and record their exact results in the [PR Verification table](docs/conventions/pull-requests.md#verification).
9. Push the branch. Open a PR to `main` for new work, or update the existing PR.
10. Address review comments and rerun the affected commands.
11. After the maintainer merges the PR, remove the branch if it contains no work to preserve.

Keep each branch and PR limited to one reviewable change. Split unrelated changes into separate PRs.

A worktree is a separate checkout of the repository. If tasks run at the same time, use separate worktrees.

## Conventions

Read the matching convention before writing or updating an artifact. If a skill gives a different template or file path, use the project convention.

| Work | Convention |
| --- | --- |
| Markdown, English prose, code comments, and agent replies | [Markdown and English prose](docs/conventions/markdown-and-prose.md) |
| Branch names | [Branches](docs/conventions/branches.md) |
| Checkpoint and squash commit messages | [Commit messages](docs/conventions/commit-messages.md) |
| Module specifications | [Module specs](docs/conventions/module-specs.md) |
| Module plans and task tracking | [Module plans](docs/conventions/module-plans.md) |
| GitHub Issue bodies and task drafts in `tasks/.todo.md` | [GitHub Issues](docs/conventions/github-issues.md) |
| Issue and pull request labels | [GitHub labels](docs/conventions/github-labels.md) |
| Pull requests and suggested squash messages | [Pull requests](docs/conventions/pull-requests.md) |
