# Repository instructions

Read and follow [`CONTRIBUTING.md`](CONTRIBUTING.md) before every task in this repository.

Read [`CONSTRAINTS.md`](CONSTRAINTS.md) before writing code. Do not weaken it to make a change pass.

For product, architecture, or implementation work, also read:

- [`docs/architecture.md`](docs/architecture.md) for current scope and system behavior.
- [`docs/adr/README.md`](docs/adr/README.md) and the ADRs relevant to the change for accepted decisions.
- [`docs/technology-stack.md`](docs/technology-stack.md) for selected implementation tools.

Treat the architecture document's open proposals as unresolved. Do not turn one into an implementation decision without explicit approval.

## English prose

Before writing any English prose, read and use the [`simple-english` skill](.agents/skills/simple-english/SKILL.md).

Apply this skill without exception to Markdown, commit messages, PR titles and descriptions, code comments, and agent replies.

Preserve code, identifiers, and tool directives as the skill requires. For Go doc comments, keep the required symbol prefix and comment syntax.

## Formatting

- Never hard-wrap Markdown files.
- Never hard-wrap PR descriptions. Keep each paragraph and list item on one physical line, regardless of length.
- Wrap body prose in checkpoint commits and suggested squash messages at 72 columns.
