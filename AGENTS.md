# Repository instructions

Use these rules before each task:

- Before each task in this repository, read and follow [`CONTRIBUTING.md`](CONTRIBUTING.md).
- Before writing code, read [`CONSTRAINTS.md`](CONSTRAINTS.md).
- Do not weaken these constraints to make a change pass.

For product, architecture, or implementation work, also read:

- [`docs/architecture.md`](docs/architecture.md) for current scope and system behavior.
- [`docs/adr/README.md`](docs/adr/README.md) and relevant architecture decision records (ADRs) for accepted decisions.
- [`docs/technology-stack.md`](docs/technology-stack.md) for selected implementation tools.

Treat open proposals in the architecture document as undecided. Do not use a proposal as an implementation decision without explicit approval.

## English prose

Use these rules for all English prose:

- Before writing any English prose, read and use the [`simple-english` skill](.agents/skills/simple-english/SKILL.md).
- Apply this skill without exception to Markdown, commit messages, PR titles and descriptions, code comments, and agent replies.
- Preserve code, identifiers, and tool directives as the skill requires.
- For Go doc comments (comments that document packages or symbols), keep the required symbol prefix and comment syntax.

## Formatting

Hard wrapping inserts manual line breaks within paragraphs or list items. Use these formatting rules:

- Do not hard-wrap Markdown files.
- Do not hard-wrap PR descriptions.
- Keep each paragraph and list item in a PR description on one physical line, regardless of length.
- Wrap body prose in checkpoint commits and suggested squash messages at 72 columns.
