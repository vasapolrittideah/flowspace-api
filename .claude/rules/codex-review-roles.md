# Review roles in Codex

Claude Code runs the test-engineer, code-reviewer, security-auditor, and convention-reviewer roles in Codex with `codex exec`, so a different model family checks the work. Codex does not read this file. Do not start the Claude subagents in [`.claude/agents/`](../agents/) for these roles.

The roles need Codex CLI 0.160.0 or later and a `codex login`. Do not use an MCP server for these roles, because Codex CLI 0.154.0 removed `codex mcp-server`.

| Role | When to run it |
| --- | --- |
| test-engineer | Before you write code for a behavior change. Skip it for typo fixes and configuration-only changes. |
| code-reviewer | After each tested change and before each commit. |
| security-auditor | When the change touches secrets, authentication, authorization, or input from outside the system. Run it at the same time as code-reviewer. |
| convention-reviewer | Before you open or update a PR, and before you give a squash message. Run `git fetch origin main` first. |

## Start a review

Run `codex exec` with the Bash tool from the root of the current checkout or worktree:

```bash
codex exec -s read-only -m <model> -c 'model_reasoning_effort="<effort>"' -o <report-file> "<prompt>" < <input-file> > <log-file> 2>&1
```

- Always pass `-s read-only`.
- Read `model` and `model_reasoning_effort` from the role file in [`.codex/agents/`](../../.codex/agents/), and pass them with `-m` and `-c`.
- Put the report file, the input file, and the log file in the scratchpad directory, not in the working tree. Codex writes its final report to the report file.
- In the prompt, tell Codex to read `AGENTS.md` and the role file in [`.agents/agents/`](../../.agents/agents/). Give the goal of the task, the diff scope, such as `git diff origin/main...HEAD`, and the test commands with their results. Do not add your own reasoning about the change, so that the review stays independent.
- Put long text in the input file, because `codex exec` appends standard input to the prompt. For convention-reviewer, put the branch name, the PR title, description, and labels, and the squash message there when they exist, because the read-only sandbox cannot read GitHub.
- Read the session ID from the `session id:` line of the log file. The review loop needs it.
- If the review can take minutes, run it in the background. Do not switch branches in the checkout while a review runs, because Codex reads the files there.

## Review loop

1. Fix each Critical and Required finding and run the tests again. Then send the changes and the new test results to the same reviewer:

   ```bash
   codex exec resume <session-id> -m <model> -c 'model_reasoning_effort="<effort>"' -c 'sandbox_mode="read-only"' -o <report-file> "<prompt>" > <log-file> 2>&1
   ```

   `codex exec resume` does not keep the model, effort, or sandbox of the session, and it ignores standard input when you pass the prompt as an argument. Pass the three settings again, and put all new text in the prompt. Do not start a new session for the same change.
2. If a finding misreads a convention, do not change the work. Quote the exact rule text in the reply and ask the reviewer to quote the words that support the finding.
3. Treat the review as passed when the verdict is `APPROVE` and no Critical or Required finding is left. Optional findings and nits do not block a commit, a PR, or a squash message.
4. If the review does not pass after 3 rounds, stop and ask the user.
5. If `codex` is not installed or not logged in, stop and tell the user to install Codex CLI 0.160.0 or later and run `codex login`. If Codex says that a model is not supported, tell the user to run `codex update`, because older versions do not know the newer models. Do not fall back to the Claude subagents without approval.
