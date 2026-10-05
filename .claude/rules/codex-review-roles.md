# Review roles in Codex

Claude Code runs the test-engineer, code-reviewer, security-auditor, convention-reviewer, writing-reviewer, spec-conformance-reviewer, planning-reviewer, migration-reviewer, infra-reviewer, and contract-reviewer roles in Codex with `codex exec`, so a different model family checks the work. Codex does not read this file. Do not start the Claude subagents in [`.claude/agents/`](../agents/) for these roles.

The roles need Codex CLI 0.160.0 or later and a `codex login`. Do not use an MCP server for these roles, because Codex CLI 0.154.0 removed `codex mcp-server`.

| Role | When to run it |
| --- | --- |
| test-engineer | Before you write code for a behavior change. Skip it for typo fixes and configuration-only changes. |
| code-reviewer | After each tested change and before each commit. |
| security-auditor | When the change touches secrets, authentication, authorization, or input from outside the system. Run it at the same time as code-reviewer. |
| convention-reviewer | Before you open or update a PR, and before you give a squash message. Run `git fetch origin main` first. |
| writing-reviewer | Before you open or update a PR, and before you give a squash message. Run it at the same time as convention-reviewer. |
| spec-conformance-reviewer | Before you open or update a PR for a task of a module plan. Run it at the same time as convention-reviewer. |
| planning-reviewer | Before you show a specification, a plan with its Issue drafts, or an ADR to the maintainer. Run it again before you open or update a PR that adds or changes a file in `docs/specs/`, `tasks/`, or `docs/adr/`, at the same time as convention-reviewer. For a plan PR, also give it the created Issues and the milestone. |
| migration-reviewer | When the change adds or changes a file under `services/*/db/migrations/`. Run it at the same time as code-reviewer. |
| infra-reviewer | When the change adds or changes a file under `deploy/` or `.github/workflows/`, the `Tiltfile`, the `cluster:*` tasks in `Taskfile.yaml`, or a local operations script in `scripts/`, such as `scripts/*-local.sh`. Run it at the same time as code-reviewer. |
| contract-reviewer | When the change adds or changes a file under `contracts/`, or `buf.yaml` or `buf.gen.yaml`. Run it at the same time as code-reviewer. |

## Start a review

Stage the change first. Then run [`scripts/codex-review.mjs`](../../scripts/codex-review.mjs) with the Bash tool:

```bash
node scripts/codex-review.mjs <role> --out <scratchpad>/<name> --prompt-file <prompt-file> [--input <input-file>]
```

- The script reads `model` and `model_reasoning_effort` from the role file in [`.codex/agents/`](../../.codex/agents/), runs `codex exec` with a read-only sandbox, and closes standard input when there is no input file.
- It writes `<name>.md` with the report and `<name>.log` with the Codex output, and it prints the session ID and the verdict. Put the prompt file, the input file, and the output in the scratchpad directory, not in the working tree.
- On `APPROVE`, it stamps the staged tree. The [`git-guard` hook](../../scripts/git-guard.mjs) blocks `git commit` when the staged tree has no `code-reviewer` stamp, or no `migration-reviewer` stamp for a staged migration. A later edit changes the staged tree, so it needs a new review.
- In the prompt, tell Codex to read `AGENTS.md` and the role file in [`.agents/agents/`](../../.agents/agents/). Give the goal of the task, the diff scope, such as `git diff --cached` or `git diff origin/main...HEAD`, and the test commands with their results. Do not add your own reasoning about the change, so that the review stays independent.
- Put long text in the input file. For convention-reviewer and writing-reviewer, put the branch name, the PR title, description, and labels, and the squash message there when they exist, because the read-only sandbox cannot read GitHub. For spec-conformance-reviewer, put the module ID and the GitHub Issue of the task there, for the same reason. For planning-reviewer, put the created Issues, the milestone, and the PR title and labels there. For convention-reviewer and writing-reviewer, also pass the PR description file with `--stamp-file`, so that the script stamps its exact text.
- Before convention-reviewer and writing-reviewer, run `node scripts/check-pr-metadata.mjs` with the PR title, labels, description file, and squash message file that exist. Fix each finding, and put the output of the script in the input file. Both reviewers skip the rules that the script checks.
- If the review can take minutes, run it in the background. Do not switch branches in the checkout while a review runs, because Codex reads the files there.

## What the hook blocks

The [`git-guard` hook](../../scripts/git-guard.mjs) runs before each Bash command and blocks these commands:

- `git commit` without a stamp for the staged tree, as stated above. It must run as its own command, without `cd`, and without `-a`, `-i`, `-o`, `-p`, or paths.
- `git cherry-pick`, `git revert`, and `git merge` without `--no-commit`, `git rebase`, `git am`, and `git pull` without `--ff-only`, because they create commits that skip the review. `--abort` is allowed, and so is a merge of `main` or `origin/main`, because its commits already passed review.
- `git push` to `main`, or to a branch whose pull requests are all merged or closed. It must run as its own command.
- `gh pr merge`, because the maintainer merges pull requests.
- `gh pr create` and `gh pr edit` without a convention-reviewer stamp and a writing-reviewer stamp for the tree of `HEAD`. When the command passes a description, it must use `--body-file` with the same file that both reviews stamped.

## Review loop

1. Fix each Critical and Required finding, run the tests again, and stage the result. Then send the changes and the new test results to the same reviewer:

   ```bash
   node scripts/codex-review.mjs <role> --out <scratchpad>/<name-2> --prompt-file <prompt-file> --resume <session-id>
   ```

   `codex exec resume` ignores standard input, so put all new text in the prompt file. Do not start a new session for the same change.
2. If a finding misreads a convention, do not change the work. Quote the exact rule text in the reply and ask the reviewer to quote the words that support the finding.
3. Treat the review as passed when the verdict is `APPROVE` and no Critical or Required finding is left. Optional findings and nits do not block a commit, a PR, or a squash message.
4. If the review does not pass after 3 rounds, stop and ask the user.
5. If `codex` is not installed or not logged in, stop and tell the user to install Codex CLI 0.160.0 or later and run `codex login`. If Codex says that a model is not supported, tell the user to run `codex update`, because older versions do not know the newer models. Do not fall back to the Claude subagents without approval.
