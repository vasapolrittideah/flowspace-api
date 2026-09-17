# Codex custom agents

These TOML files register the roles in `.agents/agents/` as project agents for Codex. Each agent reads its Markdown role file before starting work. Edit the Markdown file to change the role instructions.

Ask Codex to delegate a task to an agent by name:

```text
Use the code-reviewer subagent to review this branch.
Use the security-auditor subagent to audit this change.
Use the test-engineer subagent to find gaps in the tests.
```

For a review with all three agents, use this prompt:

```text
Review this branch with three subagents: code-reviewer, security-auditor, and test-engineer. Have each agent report findings without changing files. Wait for all three, then summarize their findings with file paths and line numbers.
```

The files do not set a model or reasoning effort. Codex uses the settings from the parent session or the configured subagent defaults. Agents start when the user or applicable project instructions request delegation.

See the [official OpenAI documentation](https://learn.chatgpt.com/docs/agent-configuration/subagents#custom-agents) for the custom agent format.
