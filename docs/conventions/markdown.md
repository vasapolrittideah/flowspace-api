# Markdown and English prose conventions

These rules keep Markdown and English prose consistent across repository artifacts. Follow the Markdown rules when you write or update a Markdown file. Follow the English prose rules whenever you write English. This includes Markdown, commit messages, PR titles and descriptions, code comments, and agent replies.

## Rules

### Markdown

- Hard wrapping inserts manual line breaks within paragraphs or list items. Do not hard-wrap Markdown files. Keep each paragraph and list item on one physical line, regardless of length.
- Give each bullet one main point. Keep its conditions, explanations, and exceptions in the same bullet. Do not split a bullet just because it has multiple sentences.
- Use separate bullets for rules that readers can follow independently.

### English prose

Before writing English prose, read the [`simple-english` skill](../../.agents/skills/simple-english/SKILL.md) and the [`humanizer` skill](../../.agents/skills/humanizer/SKILL.md). First, use Simple English to make the text easy to understand with common words and short sentences. Then, use Humanizer to make it read naturally while keeping the Simple English rules.

Apply both skills without exception. Preserve code, identifiers, and tool directives as the skills require. For Go doc comments, keep the required symbol prefix and comment syntax.

## Examples

This Markdown bullet keeps its condition and action on one physical line:

```text
- If `task markdown:check` fails, fix the Markdown issue and rerun the command.
```

Use a short, active English sentence such as `Run the relevant tests before review.`
