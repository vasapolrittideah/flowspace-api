# Review roles

The agent that writes a change runs the code-reviewer, test-reviewer, security-auditor, convention-reviewer, writing-reviewer, spec-conformance-reviewer, planning-reviewer, migration-reviewer, infra-reviewer, and contract-reviewer roles with [`.agents/scripts/review.mjs`](../scripts/review.mjs). These rules apply to every agent that writes a change. `AGENTS.md` links this file, and Claude Code loads it through the symbolic link `.claude/rules/review-roles.md`.

[`.agents/review-roles.json`](../../.agents/review-roles.json) gives each role an entry with its runner, model, and effort. A runner is a module in [`.agents/scripts/review-runners/`](../scripts/review-runners/) that starts one agent CLI in a read-only mode that denies secrets. Before a review starts, the script makes sure that the runner exists and accepts the effort. The CLI of the runner rejects a model that it does not know. The `codex` runner runs `codex exec` and needs Codex CLI 0.160.0 or later and a `codex login`. The `claude` runner runs `claude -p` and needs a logged-in Claude Code CLI. Do not use an MCP server for these roles, because Codex CLI 0.154.0 removed `codex mcp-server`.

| Role | When to run it |
| --- | --- |
| code-reviewer | After each tested change and before each commit. |
| test-reviewer | When the staged change adds or changes a Go file outside generated code. Run it at the same time as code-reviewer. Plan the tests yourself, and write each test before its code with test-driven development. |
| security-auditor | When the change touches secrets, authentication, authorization, or input from outside the system. Run it at the same time as code-reviewer. |
| convention-reviewer | Before you open or update a PR. Run `git fetch origin main` first. |
| writing-reviewer | Before you open or update a PR. Run it at the same time as convention-reviewer. |
| spec-conformance-reviewer | Before you open or update a PR for a task of a module plan. Run it at the same time as convention-reviewer. |
| planning-reviewer | Before you show a specification, a plan with its Issue drafts, or an ADR to the maintainer. Run it again before you open or update a PR that adds or changes a file in `docs/specs/`, `tasks/`, or `docs/adr/`, at the same time as convention-reviewer. For a plan PR, also give it the created Issues and the milestone. |
| migration-reviewer | When the change adds or changes a file under `services/*/db/migrations/`. Run it at the same time as code-reviewer. |
| infra-reviewer | When the change adds or changes a file under `deploy/` or `.github/workflows/`, the `Tiltfile`, the `cluster:*` tasks in `Taskfile.yaml`, or a local operations script in `scripts/`, such as `scripts/*-local.sh`. Run it at the same time as code-reviewer. |
| contract-reviewer | When the change adds or changes a file under `contracts/`, or `buf.yaml` or `buf.gen.yaml`. Run it at the same time as code-reviewer. |

## Start a review

Stage the change first. Then run [`.agents/scripts/review.mjs`](../scripts/review.mjs) in a shell:

```bash
node .agents/scripts/review.mjs <role> --out <scratchpad>/<name> --prompt-file <prompt-file> [--input <input-file>]
```

- The script reads the line of the role from `.agents/review-roles.json`, runs its runner, and closes standard input when there is no input file.
- It writes `<name>.md` with the report and `<name>.log` with the runner output, and it prints the runner, the model, the session ID, and the verdict. Put the prompt file, the input file, and the output in a temporary directory outside the working tree, such as the scratchpad directory of Claude Code. Do not name the prompt file `<name>.md`, because the script deletes an earlier report with that name before the review starts.
- On `APPROVE`, it stamps the staged tree. The [`git-guard` hook](../scripts/git-guard.mjs) blocks `git commit` when the staged tree has no `code-reviewer` stamp, no `test-reviewer` stamp for a staged Go file, or no `migration-reviewer` stamp for a staged migration. A later edit changes the staged tree, so it needs a new review.
- In the prompt, tell the reviewer to read `AGENTS.md` and the role file in [`.agents/agents/`](../../.agents/agents/). Give the goal of the task, the diff scope, such as `git diff --cached` or `git diff origin/main...HEAD`, and the test commands with their results. Do not add your own reasoning about the change, so that the review stays independent.
- Put long text in the input file. For convention-reviewer and writing-reviewer, put the branch name and the PR title, description, and labels there when they exist, because the read-only sandbox cannot read GitHub. For spec-conformance-reviewer, put the module ID and the GitHub Issue of the task there, for the same reason. For planning-reviewer, put the created Issues, the milestone, and the PR title and labels there. For test-reviewer, put your test plan and the red results there. A red result is the run where a new test failed before its code existed, with the assertion that failed. For convention-reviewer and writing-reviewer, also pass the PR description file with `--stamp-file`, so that the script stamps its exact text.
- Before convention-reviewer and writing-reviewer, run `node scripts/check-pr-metadata.mjs` with the PR title, labels, and description file that exist. Fix each finding, and put the output of the script in the input file. Both reviewers skip the rules that the script checks.
- If the review can take minutes, run it in the background. Do not switch branches in the checkout while a review runs, because the reviewer reads the files there.

## What the hook blocks

In Claude Code and Codex, the [`git-guard` hook](../scripts/git-guard.mjs) runs before each shell command. `.claude/settings.json` and `.codex/hooks.json` register it. Codex runs the hook only after the user trusts it with `/hooks`, and it asks again after each change to `.codex/hooks.json`. The hook enforces only the rules that the GitHub ruleset on `main` cannot enforce, and blocks these commands:

- `git commit` without the stamps for the staged tree, as stated above. Generated Go code under `gen/` or a `sqlc/` directory needs no `test-reviewer` stamp. It must run as its own command, without `cd`, and without `-a`, `-i`, `-o`, `-p`, or paths.
- `gh pr merge`, because the maintainer merges pull requests.
- Each `gh stack` command, because the extension creates and merges PRs without these checks, as [ADR-0040](../../docs/adr/0040-automatic-builds-run-one-subagent-for-each-ready-issue.md) states.
- `gh pr create` and `gh pr edit` without a convention-reviewer stamp and a writing-reviewer stamp for the tree of `HEAD`. When the command passes a description, it must use `--body-file` with the same file that both reviews stamped. When the branch changes a file in `docs/specs/`, `tasks/`, or `docs/adr/`, the command also needs a planning-reviewer stamp for the tree of `HEAD`.

Other agents do not run this hook. An agent without the hook, or with a hook that it does not trust yet, must follow the same rules, and must not commit or open a PR without the stamps that the hook requires.

## Review loop

1. Fix each Critical and Required finding, run the tests again, and stage the result. Then send the changes and the new test results to the same reviewer:

   ```bash
   node .agents/scripts/review.mjs <role> --out <scratchpad>/<name-2> --prompt-file <prompt-file> --resume <session-id>
   ```

   A resumed review ignores standard input, so put all new text in the prompt file. Do not start a new session for the same change.
2. If a finding misreads a convention, do not change the work. Quote the exact rule text in the reply and ask the reviewer to quote the words that support the finding.
3. Treat the review as passed when the verdict is `APPROVE` and no Critical or Required finding is left. Optional findings and nits do not block a commit or a PR.
4. If the review does not pass after 3 rounds, stop and ask the user.
5. If the CLI of a runner is not installed or not logged in, stop and tell the user to install it and log in. For `codex`, that is Codex CLI 0.160.0 or later and `codex login`. If the CLI says that a model is not supported, tell the user to update the CLI, because older versions do not know the newer models. Do not change the runner, the model, or the effort of a role without approval.
