# Convention file conventions

This convention defines the sections of each file in `docs/conventions/` and how to write each section.

## Template

```markdown
# <Topic> conventions

<what the convention covers>

## Template

<the fixed format of the artifact and how to write each part, if any>

## Rules

<rules that apply to the whole artifact or to its life cycle>

## <special case>

<a format or procedure for one situation, if any>

## Reference

<tables that readers look up while they write, if any>

## Examples

<artifacts that follow the convention, if any>
```

### Title

- Write `# <Topic> conventions` in sentence case, such as "Commit message conventions".

### Opening paragraph

- Write one paragraph that starts with "This convention".

### Template section

- Omit this section when the artifact has no fixed format.
- Start with one optional sentence that introduces the format. Then put the format in a code block with the language of the artifact, such as `markdown` or `text`.
- In the code block, write fixed text as it appears in the artifact, such as `## Context`. Write each variable part as a placeholder in angle brackets.
- Write each placeholder as a lowercase noun phrase without a final period. State only what the part is, such as `<facts and limits that exist before the decision>`. For an optional part, end the placeholder with `, if any`.
- After the code block, write one `###` subsection for each part, in template order, even when the part has only one rule. Use the name of the part from the template as the heading, such as "Context" or "Title". If the name repeats another heading in the convention file, add "section" after it, such as "Rules section".
- In each subsection, write only bullets, with one rule in each bullet. Start each bullet with a verb, such as "Write", "Put", "Use", "Link", or "Omit". If a rule has a condition, state the condition first.
- Order the bullets in each subsection: placement and format first, then the order and structure of the content, then allowed values and links, and then the rules that start with "Do not".
- If a part contains a table, add a `####` subsection for the table under the part. Write bullets for the whole table, and then a table with the columns `Column` and `How to write` that has one row for each column of the described table.
- Do not repeat the meaning from the placeholder in the subsection.

### Rules section

- Put a rule about one part of the artifact in the subsection of that part under `Template`.
- Group the rules under `###` subsections when they cover several topics.

### Special-case sections

- Name the situation in the heading, such as the [gap comment](github-issues.md#gap-comment) in the Issue convention.
- If the special case has a fixed format, write the format and its parts as the [Template section](#template-section) states.

### Reference section

- Give each table a `###` heading, such as the [commit types](commit-messages.md#types).
- Link to the table from each rule that uses it.

### Examples section

- Show a whole artifact or one part of it. State what each example shows in the sentence before it.

## Rules

- Use the sections in the template order. Omit a section that has no content, except `Rules`.
- Follow the [Markdown and English prose conventions](markdown-and-prose.md) in each convention file.
- Add a row for each convention file to the convention table in the [agent instructions](../../AGENTS.md#conventions), in the group of the work that it covers.
