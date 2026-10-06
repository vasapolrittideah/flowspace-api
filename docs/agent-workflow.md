# Agent workflow

This document explains how AI agents write and review changes in Flowspace. An agent is an AI coding tool, such as Claude Code or Codex, that reads files and runs commands. The [review roles rules](../.agents/rules/review-roles.md) give the commands and the order that an agent follows. This document explains the structure of the workflow and the reasons for it.

## Workflow pattern

The workflow uses three patterns from the article [Building effective agents](https://www.anthropic.com/engineering/building-effective-agents) by Erik Schluntz and Barry Zhang of Anthropic. The article names five workflow patterns for systems that use large language models (LLMs). Flowspace uses these three:

| Pattern | Definition in the article | Use in Flowspace |
| --- | --- | --- |
| Evaluator-optimizer | One LLM call generates a response, and another LLM call evaluates it and gives feedback in a loop. | The author agent writes a change. A reviewer reads the change and gives findings and a verdict. The author agent fixes the findings and sends the change to the same reviewer session again, for at most 3 rounds. |
| Routing | The system classifies an input and sends it to a specialized follow-up task. | The author agent selects the reviewer roles from the changed files and the stage of the work. It follows a table in the review roles rules that names each role and when it runs. For example, a change under `services/*/db/migrations/` needs `migration-reviewer`. Some rows need judgment, such as whether a change touches authorization. The hook enforces only some rows, as [Approvals and gates](#approvals-and-gates) states. |
| Parallelization by sectioning | The system splits a task into independent subtasks and runs them at the same time. | Reviewers with different responsibilities run at the same time, such as `code-reviewer` and `security-auditor`, or `convention-reviewer` and `writing-reviewer`. |

The author agent is the agent that writes the change. A reviewer is an agent that runs one review role. A reviewer can read files, but it cannot change them. Its report ends with a verdict, which is `APPROVE` or `REQUEST CHANGES`.

The maintainer is the person who reviews and merges each pull request (PR). The maintainer also connects the steps of the work. The maintainer starts each step, such as a specification, a plan, or the next task. Before the next step starts, the maintainer makes sure that its result is correct.

## Comparison with orchestrator-workers

The same article also describes the orchestrator-workers pattern. In this pattern, a central LLM divides a task into subtasks while it works. It sends the subtasks to worker LLMs, and then it combines their results. The article states that the orchestrator selects the subtasks from the input, so it does not define them before the work starts. Its examples include coding tasks that change many files and search tasks that collect information from many sources.

Flowspace does not use orchestrator-workers to write a change. The main difference is how each workflow divides the work:

| Aspect | Flowspace workflow | Orchestrator-workers |
| --- | --- | --- |
| Choice of subtasks | Review roles that the project defined before the work, with a table of rules for when each role runs | Subtasks that the orchestrator selects from the input during the work |
| Work of the other agents | They evaluate the change of the author agent and give a verdict. They cannot change files. | They do the subtasks that the orchestrator gives them. |
| Result | A verdict for each role | A combined result that the orchestrator makes from the outputs of the workers |
| Main benefit | Independent review of each change | Complex tasks whose subtasks cannot be predicted before the work starts |
| Main risk | More time for each change, because of the review rounds | Wrong subtasks, and results that are hard to trace to a subtask |

If workers write code, they can also change the same files at the same time. This risk applies to coding tasks, and the article does not use it to define the pattern.

Flowspace keeps one author agent for each change, because each PR holds one reviewable change. Independent tasks can still run at the same time. In that case, each task uses its own author agent, its own branch, and its own worktree, as the [agent instructions](../AGENTS.md#git-workflow) state. Each task then goes through the same review loop.

## Agents and files

Claude Code and Codex can each be the author agent. Each review role can run in either agent with any model that the agent supports. The files have these roles:

| Path | Agent | Contents |
| --- | --- | --- |
| `AGENTS.md` | All agents | Repository instructions. Codex reads this file. |
| `CLAUDE.md` | Claude Code | A symbolic link to `AGENTS.md`, because Claude Code reads this file name |
| `.agents/rules/review-roles.md` | All agents | The rules for when and how to run each review role. `AGENTS.md` links this file. |
| `.agents/review-roles.json` | All agents | The runner, model, and effort of each review role |
| `.agents/agents/<role>.md` | All agents | The task, process, and report format of each review role |
| `.agents/skills/`, `.agents/commands/`, `.agents/references/` | All agents | Shared skills, commands, and reference guides |
| `.claude/rules/review-roles.md` | Claude Code | A symbolic link to `.agents/rules/review-roles.md`, so that Claude Code loads the rules |
| `.claude/commands/`, `.claude/references/` | Claude Code | Symbolic links to the shared directories in `.agents/` |
| `.claude/skills/` | Claude Code | A directory with one symbolic link for each skill in `.agents/skills/` |
| `.claude/settings.json` | Claude Code | The hook that runs `scripts/git-guard.mjs` before each shell command |
| `.codex/hooks.json` | Codex | The hook that runs `scripts/git-guard.mjs` before each shell command |
| `scripts/review.mjs` | All agents | The script that runs one review role and records its approval |
| `scripts/review-runners/<runner>.mjs` | One runner each | The module that starts the CLI of one agent in a read-only mode that denies secrets |
| `scripts/git-guard.mjs` | Claude Code and Codex | The hook script that blocks a commit or a PR without the required approvals |

## Review roles and runners

A runner is a module in `scripts/review-runners/` that starts one agent CLI for a review. The `codex` runner starts `codex exec`, and the `claude` runner starts `claude -p`. Each runner gives the reviewer read access to the repository. It denies reads of home credentials and local secrets, and it denies writes to the repository and to its Git directories.

`.agents/review-roles.json` gives each role an entry with its runner, model, and effort. To move a role to another agent or model, the maintainer changes only that entry. Before a review starts, `scripts/review.mjs` stops when the runner does not exist or does not accept the effort. The CLI of each runner rejects a model that it does not know, so the script does not make sure that model names are valid.

To support another agent, a contributor adds files and changes no existing runner:

1. If the agent does not read `AGENTS.md` itself, give it the repository instructions in that file.
2. If the agent supports hooks before shell commands, register `scripts/git-guard.mjs` to run before each shell command.
3. If the agent will run review roles, add `scripts/review-runners/<runner>.mjs`. The module exports the command, the efforts that the CLI accepts, `args()`, and `sessionID()`, as `scripts/review.mjs` describes. If the CLI prints the report and does not write the report file, the module also exports `report()`.

## Approvals and gates

An approval stamp is a file that records that a reviewer approved one exact version of the change. When a reviewer gives `APPROVE`, `scripts/review.mjs` writes a stamp for the staged tree. The staged tree is the Git tree object of the files that `git add` prepared for the next commit. The script writes the stamp to `<git-common-dir>/reviews/<role>/<tree>`, where `<git-common-dir>` is the Git directory that all worktrees of the repository share. A later edit gives a different tree, so the edit needs a new review.

A reviewer of a PR description also approves its exact text. If the author agent passes the description file to the script with `--stamp-file`, the script also writes a stamp for the hash of that text.

Claude Code and Codex both register the `git-guard` hook to run before each shell command. Codex runs it only after the user trusts it, as [Limits](#limits) states. The hook reads the stamps:

- The hook blocks a commit when the staged tree has no `code-reviewer` stamp. If the staged tree is the tree of `HEAD`, the hook allows the commit. A commit that only changes the message needs no new review.
- The hook blocks a commit that changes a migration when the staged tree has no `migration-reviewer` stamp.
- The hook blocks `gh pr create` and `gh pr edit` when the tree of `HEAD` has no `convention-reviewer` stamp or no `writing-reviewer` stamp. `gh pr create` must pass the description with `--body-file`. `gh pr edit` must use `--body-file` when it changes the description. Both roles must have a stamp for the exact text of that file.
- The hook blocks `gh pr create` and `gh pr edit` when the branch changes a file in `docs/specs/`, `tasks/`, or `docs/adr/`, and the tree of `HEAD` has no `planning-reviewer` stamp.
- The hook blocks `gh pr merge` because the maintainer merges each PR.

A script decides each gate, and an LLM does not. A reviewer gives a verdict, but only the stamp files and the hook decide whether a commit or a PR can continue. The hook finds forgotten steps. It does not stop a person or an agent that bypasses it on purpose.

## Limits

The current workflow has these limits:

- Codex runs a project hook only after the user trusts it with `/hooks`. In Codex CLI 0.160.0, `/hooks` does not list the project hook, so Codex skips it without a warning. Until the user can trust the hook, Codex must follow the review rules without it.
- An agent without a hook for `git-guard` gets no block for a commit or a PR without approval. It must follow the review rules on its own.
- The `claude` runner needs the operating system sandbox of Claude Code. Claude Code does not sandbox Bash on native Windows, so the runner stops there.

## Sources

- [Building effective agents](https://www.anthropic.com/engineering/building-effective-agents)
