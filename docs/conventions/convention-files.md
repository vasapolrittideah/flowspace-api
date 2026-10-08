# Convention file conventions

This convention defines the file, sections, and wording of each convention file in `docs/conventions/`.

## Template

A convention file has a [Title](#title), a [Scope](#scope), a [Template section](#template-section), a [Rules section](#rules-section), [Special cases](#special-cases), [Differences from the skill](#differences-from-the-skill), [Reference](#reference), and [Examples](#examples).

```markdown
# <topic> conventions

<scope of the convention>

## Template

<format of the artifact and the rules for each part, if any>

## Rules

<rules for the whole artifact, its file, and its life cycle>

## <special case>

<procedure or format for 1 situation, if any>

## Differences from the <skill name> skill

<skill rules that this convention replaces, if any>

## Reference

<tables that readers look up while they write, if any>

## Examples

<artifacts or parts of artifacts that show the convention, if any>
```

### Title

- Write `# <topic> conventions`, and follow the [Format and content](markdown-and-english-prose.md#format-and-content) rules for the case of the heading.
- Write the topic as the singular name of the artifact, such as "Commit message conventions". If the convention covers several artifacts, join their names with "and", such as "Markdown and English prose conventions". For 3 or more names, separate the names with commas and put "and" before the last name.

### Scope

- Write the scope as 1 or 2 paragraphs.
- Start the first paragraph with "This convention", and state in it the artifact and what the convention controls, such as its format, name, or life cycle.
- If the artifact has a fixed location, name the location in the first paragraph, such as `docs/adr/`.
- If another document owns the purpose of the artifact or the decision behind the convention, link to that document in the second paragraph. If no such document exists, omit the second paragraph.
- Follow the [Glossary entry](glossary-entries.md) conventions for the terms of the convention.
- Do not add other paragraphs to the scope.

### Template section

- If the artifact has no fixed format, omit this section.
- Start with 1 sentence that names every part of the artifact in template order. If a part is outside the body, such as the title of an Issue or a pull request, name it first.
- Name each part with a short noun phrase. If the part is a section with a fixed heading, use the heading text, such as "Context".
- After the sentence, put the format in a code block with the language of the artifact, such as `markdown` or `text`.
- In the code block, write fixed text as it appears in the artifact, such as `## Context`. Write each variable part as a placeholder in angle brackets.
- If a part can repeat, show it once in the code block, such as 1 acceptance criterion. State how often it can repeat in the subsection of the part.
- Write each placeholder as a lowercase noun phrase without a final period. State only what the part is, such as `<facts and limits that exist before the decision>`.
- Keep the capitals of names and abbreviations in a placeholder, such as `<module ID>` or `<Issue number>`.
- If a placeholder shows the format of a value, write the format in uppercase letters, such as `<NNNN>` for a 4-digit number.
- For an optional part, end the placeholder with `, if any`. If the optional part has a heading, put `, if any` in the first placeholder under the heading.
- After the code block, write 1 `###` subsection for each part, in the order of the first sentence.
- Name each subsection with the name of its part from the first sentence, and capitalize the first word, such as "Issue footers".
- If the name of a subsection repeats another heading in the convention file, add a word that tells the headings apart, such as "Rules section" or "Result gaps".
- Write only bullets in each subsection.
- If a part contains a table, add a `####` subsection for the table under the part. Write bullets for the whole table, and then a table with the columns `Column` and `How to write` that has 1 row for each column of the described table.
- Write at least 1 rule in each subsection, such as the format, length, source, or links of the part.
- Do not repeat the meaning from the placeholder in the subsection.

### Rules section

- Write only bullets in this section.
- Put the rules for the file, location, numbering, and life cycle of the artifact here. Also put the rules that cover more than 1 part here.
- Include exactly 1 bullet that states how existing artifacts follow a change to the convention. Choose one of these approaches: update every existing artifact in the same PR, or apply the change only to new artifacts and to artifacts that a later PR changes. Put the bullet under [Changes](#changes) when the section has `###` headings. Other bullets under [Changes](#changes) can cover changes to an artifact after it exists.
- If the section has more than 5 bullets, group them under `###` headings. Use only these headings, in this order, and omit a heading that has no bullets:
  1. `Naming and location` for the name, number, and location of the artifact.
  2. `Format and content` for the sections and fields of the artifact and the rules that cover several parts.
  3. `Workflow` for the steps to create and review the artifact and to make sure that it follows its convention, and how many artifacts to write.
  4. `Links and tracking` for the indexes, tables, links, labels, milestones, and GitHub relationships that must match the artifact.
  5. `Status and approval` for the status values of the artifact and its approval.
  6. `Changes` for changes to an artifact after it exists, and for how existing artifacts follow a change to the convention.
- If a bullet fits several of these headings, put it under the first match in this order:
  1. `Changes`
  2. `Status and approval`
  3. `Links and tracking`
  4. `Workflow`
  5. `Naming and location`
  6. `Format and content`
- If the convention covers several kinds of the artifact, such as queries and migrations, name each `###` heading after 1 kind instead. Order the bullets under each kind by the heading order above, without more headings. Put the bullet about how existing artifacts follow a change under a last `### Changes` heading.
- If the section has 5 bullets or fewer, do not add `###` headings.
- Do not put a rule about 1 part in this section. Put the rule in the subsection of that part.

### Special cases

- If a procedure or format applies only to some artifacts of the kind, such as the [Final Prove task PR](pull-requests.md#final-prove-task-pr) in the pull request convention, add 1 section for that situation. Put a procedure that applies to every artifact of the kind under [Workflow](#workflow) in the [Rules section](#rules-section).
- Name the situation in the heading as a noun phrase.
- Start with paragraphs that state when the situation applies and the procedure to follow.
- If rules apply to the whole situation, write them as bullets after the paragraphs.
- If the situation has a fixed format, write the format after the bullets as the [Template section](#template-section) states. Use `###` subsections for the parts.
- If a situation contains a smaller situation, write the smaller situation as a `###` subsection with the same layout. Use `####` subsections for its parts.
- Do not put a situation inside a smaller situation.

### Differences from the skill

- If a skill in `.agents/skills/` covers the artifact and gives a different file path, format, section, or workflow, add this section.
- Use the folder name of the skill in the heading, such as "Differences from the spec-driven-development skill".
- Start with 1 sentence that links the `SKILL.md` file of the skill and states that this convention applies where the skill and this convention differ.
- Write 1 bullet for each difference. Start with the project rule, and then state the skill rule that it replaces, such as "Save the specification as `docs/specs/<module-id>.md`. The skill saves it as `SPEC-<module-id>.md`."
- Link to a rule in this convention instead of repeating the rule.

### Reference

- Put each table under its own `###` heading, such as [Types](commit-messages.md#types) in the commit message conventions.
- Write only the `###` headings and their tables in this section.
- Link to the table from each rule that uses it.
- Do not put rules in this section. Put each rule in the subsection that uses the table.

### Examples

- Write 1 sentence before each example that states what the example shows.
- If you add a counter-example, state in the sentence before it that it does not follow the convention. Name the rule that it breaks.
- Show each example in a code block with the language of the artifact. If each example is a name, such as a file name, show the examples in a table.
- If the examples show several kinds of artifacts, group them under `###` headings named after the kind, such as "Checkpoint commits".
- If an example is a GitHub item or a whole repository file, link to it instead of copying it.
- Make each example follow every rule of the current convention, except a counter-example.
- Do not use bullets for examples.

## Rules

### Naming and location

- Save the file as `docs/conventions/<name>.md`. Write the name as every word of the topic, in lowercase, separated by hyphens. Put the last noun of each name that the topic joins with "and" in plural form, and keep a noun that has no plural, such as `hexagonal-components-and-files.md` for "Hexagonal component and file" or `markdown-and-english-prose.md` for "Markdown and English prose". If the topic contains a term that a directory name in the repository shortens, use the short form, such as `adrs.md` for `docs/adr/` and `module-specs.md` for `docs/specs/`.

### Format and content

- Follow the [Markdown and English prose](markdown-and-english-prose.md) conventions in each convention file.
- Start each bullet with a verb, such as "Write", "Put", "Use", "Link", or "Omit". If the rule has a condition or applies to 1 place, state the condition or the place first, such as "In the index, use the record number". Apply this rule to every bullet outside code blocks. This rule does not cover the items of a numbered list inside a bullet.
- If a rule applies steps in order or by precedence, put a numbered list inside its bullet.
- Follow the [Format and content](markdown-and-english-prose.md#format-and-content) rules for the main point, conditions, explanations, and exceptions of each bullet.
- Order the bullets in each list by these groups. If a bullet fits several groups, put it in the earliest group:
  1. Placement and format.
  2. Order and structure of the content.
  3. Allowed values and links.
  4. Rules that start with "Do not".
- Use the sections in the template order. Omit an optional section that has no content.
- Do not add other top-level sections. Add a new section to this convention before you use it in a convention file.

### Workflow

- Write 1 convention file for each kind of artifact. If a convention already covers the artifact, add the rule to that file.

### Links and tracking

- Put each rule in 1 convention file only. In other files, link to the rule instead of repeating it.
- Add a row for each convention file to the table in the [Conventions](../../AGENTS.md#conventions) section of the repository instructions.
- Put the row in the group of the work that first needs the convention. If the work happens in several groups, use the earliest group in the table.
- In the `Work` column, write a phrase that starts with a verb ending in "-ing" and names the artifact, such as "Writing or updating module plans in `tasks/<module-id>.md`". If the artifact has a fixed location, name the location.
- In the `Convention` column, link to the file. Use the topic as the link text, with the plural forms of the file name and without short forms, such as "Commit messages" or "Module specifications".
- In the group, put the rows in the order in which the work happens.

### Changes

- Apply a change of this convention only to new convention files and to convention files that a later PR changes for any reason. In that PR, make the whole changed file follow this convention. This includes the file name and the row in the agent instructions. If the PR renames the file, update every link to the file in the same PR. A PR that only updates the links to a renamed file or heading in another convention file does not count as a change of that file. The same applies to a PR that only applies 1 changed rule of this convention to other convention files.
