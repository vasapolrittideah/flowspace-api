# Markdown and English prose conventions

This convention defines Markdown and English prose rules for repository artifacts.

## Rules

### Markdown

- Use bullets for multiple rules, checks, choices, or facts that readers can follow independently. Give each bullet one main point. Keep its conditions, explanations, and exceptions in the same bullet, even when it takes multiple sentences.
- Use a numbered list when readers must follow steps in order or apply rules by precedence.
- Use a paragraph for context or an explanation that develops one idea. Keep related sentences together.
- Hard wrapping inserts manual line breaks within paragraphs or list items. Do not hard-wrap Markdown files. Keep each paragraph and list item on one physical line, regardless of length.

### English prose

- Apply these rules to Markdown, commit messages, PR titles and descriptions, and code comments.
- Before writing English prose, read the [`simple-english` skill](../../.agents/skills/simple-english/SKILL.md) and the [`humanizer` skill](../../.agents/skills/humanizer/SKILL.md).
- First, use Simple English to make the text easy to understand with common words and short sentences. Then, use Humanizer to make it read naturally while keeping the Simple English rules. Check the final text against Simple English again and fix any conflicts. Apply both skills without exception.
- Preserve code, identifiers, and tool directives as the skills require.
- For Go doc comments, keep the required symbol prefix and comment syntax.
