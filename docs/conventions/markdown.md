# Markdown and English prose conventions

## Markdown

- Hard wrapping inserts manual line breaks within paragraphs or list items. Do not hard-wrap Markdown files. Keep each paragraph and list item on one physical line, regardless of length.
- Give each bullet one main point. Keep its conditions, explanations, and exceptions in the same bullet. Do not split a bullet just because it has multiple sentences.
- Use separate bullets for rules that readers can follow independently.

## English prose

Use these rules for all English prose:

- Before writing any English prose, read the [`simple-english` skill](../../.agents/skills/simple-english/SKILL.md) and the [`humanizer` skill](../../.agents/skills/humanizer/SKILL.md). First, use Simple English to make the text easy to understand with common words and short sentences. Then, use Humanizer to make the text read naturally while keeping the Simple English rules.
- Apply both skills without exception to Markdown, commit messages, PR titles and descriptions, code comments, and agent replies.
- Preserve code, identifiers, and tool directives as the skills require. For Go doc comments (comments that document packages or symbols), keep the required symbol prefix and comment syntax.
