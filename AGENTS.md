# Repository instructions

Use these rules before each task:

- Before each task in this repository, read and follow [`CONTRIBUTING.md`](CONTRIBUTING.md).
- Before writing code, read [`CONSTRAINTS.md`](CONSTRAINTS.md). Do not weaken these constraints to make a change pass.

For product, architecture, or implementation work, also read:

- [`docs/architecture.md`](docs/architecture.md) for current scope and system behavior.
- [`docs/adr/README.md`](docs/adr/README.md) and relevant architecture decision records (ADRs) for accepted decisions.
- [`docs/technology-stack.md`](docs/technology-stack.md) for selected implementation tools.

Treat open proposals in the architecture document as undecided. Do not use a proposal as an implementation decision without explicit approval.

## Planning and task tracking

- Save each module plan as `tasks/<module-id>.md`.
- Use `tasks/.todo.md` only as the temporary source for GitHub Issue creation.
- Track tasks in GitHub Issues and task status in the repository GitHub Project.
- Delete `tasks/.todo.md` after every task exists in the GitHub Project.

## English prose

Use these rules for all English prose:

- Before writing any English prose, read the [`simple-english` skill](.agents/skills/simple-english/SKILL.md) and the [`humanizer` skill](.agents/skills/humanizer/SKILL.md). First, use Simple English to make the text easy to understand with common words and short sentences. Then, use Humanizer to make the text read naturally while keeping the Simple English rules.
- Apply both skills without exception to Markdown, commit messages, PR titles and descriptions, code comments, and agent replies.
- Preserve code, identifiers, and tool directives as the skills require. For Go doc comments (comments that document packages or symbols), keep the required symbol prefix and comment syntax.
