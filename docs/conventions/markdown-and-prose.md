# Markdown and English prose conventions

This convention defines Markdown and English prose rules for repository artifacts.

## Rules

### Markdown

- Use bullets for multiple rules, checks, choices, or facts that readers can follow independently. Give each bullet one main point. Keep its conditions, explanations, and exceptions in the same bullet, even when it takes multiple sentences. To test the choice, remove or reorder the items. If each remaining item is still clear, use bullets.
- Use a numbered list when readers must follow steps in order or apply rules by precedence.
- Use a paragraph for context or an explanation that develops one idea. Keep related sentences together. If a sentence depends on an earlier one, such as through "so", "but", or "this approach", keep both in paragraphs. An explanation can use several paragraphs. Start a new paragraph for each step, such as a limit, a rejected option, and the decision.
- Hard wrapping inserts manual line breaks within paragraphs or list items. Do not hard-wrap Markdown files. Keep each paragraph and list item on one physical line, regardless of length.

### English prose

- Apply these rules to Markdown, commit messages, PR titles and descriptions, and code comments.
- Before writing English prose, read the [`simple-english` skill](../../.agents/skills/simple-english/SKILL.md) and the [`humanizer` skill](../../.agents/skills/humanizer/SKILL.md).
- First, use Simple English to make the text easy to understand with common words and short sentences. Then, use Humanizer to make it read naturally while keeping the Simple English rules. Check the final text against Simple English again and fix any conflicts. Apply both skills without exception.
- Preserve code, identifiers, and tool directives as the skills require.
- For Go doc comments, keep the required symbol prefix and comment syntax.

### Convention files

Use these sections in this order in each file under `docs/conventions/`. Omit a section that has no content, except `Rules`.

1. A `# <Topic> conventions` title and an opening paragraph that states what the convention covers.
2. `## Template`: the fixed format, then one bullet per part in the form `- <Part>: <content and format>`. Omit this section when the artifact has no fixed format.
3. `## Rules`: the rules, grouped under `###` subsections when they cover several topics.
4. Sections for special cases, such as the gap comment in the [Issue convention](github-issues.md).
5. `## Reference`: tables that readers look up, such as the [commit types](commit-messages.md#types).
6. `## Examples`.
