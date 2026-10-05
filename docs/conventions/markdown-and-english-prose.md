# Markdown and English prose conventions

This convention defines how to format Markdown and how to write English prose in the text that the project writes. The text covers only Markdown files, code comments in every language including Protobuf, commit messages, and GitHub text. It excludes files in `.agents/` and `.claude/`, generated files, string values in code and configuration, and replies in chat. GitHub text is the text of Issues, pull requests, comments, and milestones on GitHub. Markdown text is the text of Markdown files and GitHub text. A generated file is a file that a tool writes, such as the files under `gen/`. Hard wrapping is the insertion of manual line breaks inside a paragraph or a list item. A tool directive is a comment that a tool reads, such as `//go:build` or `//go:generate`.

## Rules

### Format and content

- In Markdown text, keep each paragraph and list item on one physical line, regardless of length. Do not hard-wrap Markdown text.
- In Markdown text, write headings in sentence case. Keep the capitals of names, abbreviations, and the fixed headings that a template defines.
- Use bullets for two or more rules, checks, choices, or facts that readers can follow independently. To test the choice, remove or reorder the items. If each remaining item is still clear, use bullets. If there is only one item, write a paragraph.
- If a paragraph states the result or the context that all bullets of a list share, put it directly before the list. Do not write more than one such paragraph for a list.
- Give each bullet one main point. Keep its conditions, explanations, and exceptions in the same bullet, even when it takes several sentences.
- Use a numbered list when readers must follow steps in order or apply rules by precedence.
- Number the items of every numbered list with Arabic numerals that start at 1 and go up by 1, such as `1.`, `2.`, and `3.`. Do not use Roman numerals or letters, such as `i.`, `ii.`, or `a.`. Apply this rule to the source text. GitHub shows a numbered list inside a bullet with Roman numerals, and that display follows this rule.
- Use a paragraph for context or for an explanation that develops one idea. If a sentence depends on an earlier one, such as through "so", "but", or "this approach", keep both sentences in the same paragraph. Start a new paragraph for each new idea in an explanation, such as a limit, a rejected option, and the decision.
- In Markdown text, if each item has the same two or more attributes, such as a name and a meaning, use a table instead of bullets.
- In Markdown text, put code, identifiers, file paths, commands, field names, and GitHub labels in backticks. Also put a value in backticks when a reader types it or a system reads it exactly, such as a status value or a configuration value.
- In Markdown files, link to a tracked file with a relative path, and link to an Issue or a pull request with its full URL. In commit messages and GitHub text, link to a file with its full GitHub URL on `main`, and refer to an Issue or a pull request with its number, such as `#123`.
- In Markdown text, write link text that names the target, such as the title of the document or the name of the section.
- Use straight quotation marks and apostrophes.
- Use American spelling.
- Keep code, identifiers, tool directives, and quoted output unchanged when you edit prose.
- In Go doc comments, keep the symbol prefix and the comment syntax that Go requires.
- Write each descriptive sentence with at most 30 words. Write each procedural sentence, which tells the reader what to do, with at most 25 words.
- In Markdown text, do not put product names or ordinary words in backticks, such as PostgreSQL or Issue.
- In Markdown text, do not use bare URLs or link text such as "here" or "this link".
- In Markdown text, do not use bold or italic text.
- In Markdown text, do not put a bullet list inside another list. Only a numbered list can go inside a bullet.

### Workflow

- Before you write English prose, read the [`simple-english` skill](../../.agents/skills/simple-english/SKILL.md) and the [`humanizer` skill](../../.agents/skills/humanizer/SKILL.md). An agent reads them once in each session, and again when a skill file changes.
- Write the text with the `simple-english` skill in Plain mode. Then edit the text with the `humanizer` skill so that it reads naturally. Then check the text against the `simple-english` skill again, and fix each conflict in favor of the `simple-english` skill.
- If two rules conflict, apply the rule that comes first in this order:
  1. The rules of another convention for the artifact and the formats that a tool requires, such as a template heading, an ADR status value, a Conventional Commits subject, a Go doc comment prefix, or an attribution line that an agent harness adds.
  2. The rules in this convention.
  3. The `simple-english` skill.
  4. The `humanizer` skill.

### Changes

- Apply this convention to the text that you add or change. If you change part of a paragraph or a bullet, make the whole paragraph or bullet follow this convention. Do not change other text only to make it follow this convention, unless the PR exists for that change.

## Differences from the simple-english skill

The [`simple-english` skill](../../.agents/skills/simple-english/SKILL.md) sets the rules for English prose, but this convention applies where the two differ.

- Use the sentence limits in the [format rules](#format-and-content). The skill allows 25 words for descriptive sentences and 20 for procedural sentences.
