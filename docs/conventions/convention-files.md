# Convention file conventions

This convention defines the sections of each file in `docs/conventions/` and the form of each section.

## Template

```markdown
# <Topic> conventions

<what the convention covers>

## Template

<the fixed format of the artifact>

## Section forms

<how to write each section of the artifact>

## Rules

<rules that apply to the whole artifact or to its life cycle>

## <special case>

<a format or procedure for one situation>

## Reference

<tables that readers look up while they write>

## Examples

<artifacts that follow the convention>
```

The [section forms](#section-forms) state how to write each section.

## Section forms

Each subsection states how to write one section.

### Title and opening paragraph

- Write the title as `# <Topic> conventions` in sentence case, such as "Commit message conventions".
- Write one opening paragraph that starts with "This convention".

### Template section

- Omit this section when the artifact has no fixed format.
- Put the format in a code block with the language of the artifact, such as `markdown` or `text`. Write each variable part as a placeholder in angle brackets.
- If the convention has `Section forms`, write in each placeholder only what the part means, such as `<facts and limits that exist before the decision>`. Do not add bullets after the code block. End the section with "The [section forms](#section-forms) state how to write each section."
- If the convention has no `Section forms`, write short placeholders, such as `<type>`. After the code block, write one bullet for each part in template order, in the form `- <Part>: <meaning and form>`.

### Section forms section

- Add this section when a section of the artifact needs more than one rule, a table, or a set of allowed values. Omit it when one bullet after the template can state each part.
- Start the section with "Each subsection states how to write one section."
- Write one `###` subsection for each section of the artifact, in template order. Add a subsection for a header field, such as a status line, when it needs a form. Use the name from the template as the heading. If the name repeats a heading of the convention file, add "section" after it, such as "Rules section".
- State only how to write the section, such as its order, format, allowed values, links, and what to leave out. Do not repeat the meaning from the template placeholder.
- If a section contains a table, describe the table under a `####` heading in the subsection of that section. Use a table with the columns `Column` and `How to write`, and write one row for each column of the described table.

### Rules section

- Put a rule about one section of the artifact in `Section forms`.
- Group the rules under `###` subsections when they cover several topics.

### Special-case sections

- Name the situation in the heading, such as the [gap comment](github-issues.md#gap-comment) in the Issue convention.

### Reference section

- Give each table a `###` heading, such as the [commit types](commit-messages.md#types).
- Link to the table from each rule or form that uses it.

### Examples section

- Show a whole artifact or one part of it. State what each example shows in the sentence before it.

## Rules

- Use the sections in the template order. Omit a section that has no content, except `Rules`.
- Follow the [Markdown and English prose conventions](markdown-and-prose.md) in each convention file.
- Add a row for each convention file to the convention table in the [agent instructions](../../AGENTS.md#conventions), in the group of the work that it covers.
