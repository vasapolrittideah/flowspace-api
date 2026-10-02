# Markdown and English prose conventions

This convention defines how to format Markdown and how to write English prose in the text that the project writes. The text includes tracked files, commit messages, GitHub text, and code comments in every language, including Protobuf. It excludes files in `.agents/` and `.claude/`, generated files, and replies in chat. The formatting rules apply to Markdown files and to GitHub text, and the prose rules apply to all of the text. GitHub text is the text of Issues, pull requests, comments, and milestones on GitHub. Hard wrapping is the insertion of manual line breaks inside a paragraph or a list item. A tool directive is a comment that a tool reads, such as `//go:build` or `//go:generate`.

## Rules

### Sections and wording

- Keep each paragraph and list item on one physical line, regardless of length. Do not hard-wrap Markdown files or GitHub text. A commit message follows the line limit in the [commit message convention](commit-messages.md#body) instead.
- Write headings in sentence case. Keep the capitals of names, abbreviations, and the fixed headings that a template defines.
- Use bullets for two or more rules, checks, choices, or facts that readers can follow independently. To test the choice, remove or reorder the items. If each remaining item is still clear, use bullets.
- Give each bullet one main point. Keep its conditions, explanations, and exceptions in the same bullet, even when it takes several sentences.
- Use a numbered list when readers must follow steps in order or apply rules by precedence.
- Use a paragraph for context or for an explanation that develops one idea. If a sentence depends on an earlier one, such as through "so", "but", or "this approach", keep both sentences in the same paragraph. Start a new paragraph for each new idea in an explanation, such as a limit, a rejected option, and the decision.
- Use a table when each item has the same two or more attributes, such as a name and a meaning.
- Put code, identifiers, file paths, commands, field names, literal values, and GitHub labels in backticks.
- Link to a tracked file with a relative path. In commit messages and GitHub text, link to a file with its full GitHub URL, and refer to an Issue or a pull request with its number, such as `#123`. Relative links do not work there.
- Use straight quotation marks and apostrophes.
- Use American spelling.
- Keep code, identifiers, tool directives, and quoted output unchanged when you edit prose.
- In Go doc comments, keep the symbol prefix and the comment syntax that Go requires.
- Do not use bold or italic text.
- Do not put a bullet list inside another list. Only a numbered list can go inside a bullet.

### Workflow

- Before you write English prose for the first time in a session, read the [`simple-english` skill](../../.agents/skills/simple-english/SKILL.md) and the [`humanizer` skill](../../.agents/skills/humanizer/SKILL.md). If a skill file changes during the session, read it again.
- Write the text with the `simple-english` skill in Plain mode. Then edit the text with the `humanizer` skill so that it reads naturally. Then check the text against the `simple-english` skill again, and fix each conflict in favor of the `simple-english` skill.
- If two rules conflict, apply the rule that comes first in this order:
  1. The rules of another convention for the artifact and the formats that a tool requires, such as a template heading, an ADR status value, a Conventional Commits subject, or a Go doc comment prefix.
  2. The rules in this convention.
  3. The `simple-english` skill.
  4. The `humanizer` skill.

### Changes

- Apply this convention to the text that a PR adds or changes. If a PR changes part of a paragraph or a bullet, make the whole paragraph or bullet follow this convention. Do not change other prose only to make it follow this convention, unless the PR exists for that change.
