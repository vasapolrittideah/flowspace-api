# Review roles in Codex

Claude Code runs the test-engineer, code-reviewer, security-auditor, and convention-reviewer roles in Codex through the `codex` MCP server in [`.mcp.json`](../../.mcp.json), so a different model family checks the work. Codex does not read this file. Do not start the Claude subagents in [`.claude/agents/`](../agents/) for these roles.

| Role | When to run it |
| --- | --- |
| test-engineer | Before you write code for a behavior change. Skip it for typo fixes and configuration-only changes. |
| code-reviewer | After each tested change and before each commit. |
| security-auditor | When the change touches secrets, authentication, authorization, or input from outside the system. Run it at the same time as code-reviewer. |
| convention-reviewer | Before you open or update a PR, and before you give a squash message. Run `git fetch origin main` first. |

## Codex tool arguments

Call the `codex` tool with these arguments:

- Set `sandbox` to `read-only` in every call. Never send another value, because the argument overrides the server default.
- Set `approval-policy` to `never`.
- Read `model` and `model_reasoning_effort` from the role file in [`.codex/agents/`](../../.codex/agents/). Set `model` to its `model` value, and set `config` to `{"model_reasoning_effort": "<its value>"}`.
- Set `cwd` to the root of the current checkout or worktree.
- In `prompt`, tell Codex to read `AGENTS.md` and the role file in [`.agents/agents/`](../../.agents/agents/). Give the goal of the task, the diff scope, such as `git diff main...HEAD`, and the test commands with their output. For convention-reviewer, also give the branch name, the PR title, description, and labels, and the squash message when they exist, because the read-only sandbox cannot read GitHub. Do not add your own reasoning about the change, so that the review stays independent.

## Review loop

1. Fix each Critical and Required finding, run the tests again, and send the new test output to the same reviewer with `codex-reply` and its `threadId`. Do not start a new thread for the same change.
2. Treat the review as passed when the verdict is `APPROVE` and no Critical or Required finding is left. Optional findings and nits do not block a commit, a PR, or a squash message.
3. If the review does not pass after 3 rounds, stop and ask the user.
4. If the `codex` tool is not available, stop and tell the user how to enable it: install the Codex CLI, run `codex login`, and approve the `codex` server in [`.mcp.json`](../../.mcp.json). Do not fall back to the Claude subagents without approval.
