---
description: Commit the working tree on a branch
allowed-tools: Bash(git status:*), Bash(git diff:*), Bash(git add:*), Bash(git commit:*), Bash(git log:*), Bash(git branch:*)
---

Commit the current working tree. The message convention, the staging safety
rule and the test-state rule are in `CLAUDE.md` and are not repeated here.

1. Stop if the current branch is `main`. ADR-0013 allows no direct commits
   there.

2. Read `git status --short` and `git diff --stat`, and list untracked files
   explicitly before staging anything.

3. Stage and commit. Do not push. The co-author trailer comes from
   `.claude/settings.json`; do not write one by hand.

Unrelated changes in one commit are fine. The commit is discarded at merge, so
splitting it buys nothing.
