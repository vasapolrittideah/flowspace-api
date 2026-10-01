# Convention file conventions

This convention defines the sections of each file in `docs/conventions/` and the form of each section.

## Template

```markdown
# <Topic> conventions

<what the convention covers>

## Template

<the fixed format, then one bullet for each part>

## Section forms

### <part>

- <rule for the part>

## Rules

### <rule topic>

- <rule>

## <special case>

<format or procedure for one situation>

## Reference

### <table name>

<table>

## Examples

<artifacts that follow the convention>
```

- Title and opening paragraph: The topic and what the convention covers.
- Template section: The fixed format of the artifact and one bullet for each part.
- Section forms section: How to write each part of the template, when a part needs more than one rule.
- Rules section: Rules that apply to the whole artifact or to its life cycle.
- Special-case sections: A format or procedure for one situation.
- Reference section: Tables that readers look up while they write.
- Examples section: Artifacts that follow the convention.

The [section forms](#section-forms) state how to write each part.

## Section forms

Each subsection states what a part contains and the form in which to write it.

### Title and opening paragraph

- Write the title as `# <Topic> conventions` in sentence case, such as "Commit message conventions".
- Write one opening paragraph that starts with "This convention" and states the artifact or work that the convention covers.

### Template section

- Omit this section when the artifact has no fixed format.
- Put the format in a code block with the language of the artifact, such as `markdown` or `text`. Write each variable part as `<name>`.
- After the code block, write one bullet for each part in template order, in the form `- <Part>: <content>`.
- If the convention has `Section forms`, write one sentence in each bullet and put the detail in the forms. End the section with "The [section forms](#section-forms) state how to write each part."

### Section forms section

- Add this section when a part of the template needs more than one rule, a table, or a set of allowed values. Omit it when one bullet in `Template` states each part.
- Start the section with "Each subsection states what a part contains and the form in which to write it."
- Write one `###` subsection for each part, in template order, with the part name from the template as its heading. Write the rules for the part as bullets.
- If a part contains a table, describe the table under a `####` heading in the subsection of that part. Use a table with the columns `Column` and `How to write`, and write one row for each column of the described table.

### Rules section

- Write rules that apply to the whole artifact or to its life cycle, such as file names, numbering, section order, status, and approval. Put a rule about one part in `Section forms`.
- Group the rules under `###` subsections when they cover several topics.

### Special-case sections

- Add a `##` section for a format or procedure that applies only in one situation, such as the [gap comment](github-issues.md#gap-comment) in the Issue convention. Name the situation in the heading.

### Reference section

- Put tables that readers look up while they write, such as the [commit types](commit-messages.md#types). Give each table a `###` heading.
- Link to the table from each rule or form that uses it.

### Examples section

- Show artifacts or parts of artifacts that follow the convention. State what each example shows in the sentence before it.

## Rules

- Use the sections in the template order. Omit a section that has no content, except `Rules`.
- Follow the [Markdown and English prose conventions](markdown-and-prose.md) in each convention file.
- Add a row for each convention file to the convention table in the [agent instructions](../../AGENTS.md#conventions), in the group of the work that it covers.
