# Convention file conventions

This convention defines the file, sections, and wording of each convention file in `docs/conventions/`. A convention file tells agents and developers how to write one kind of artifact. An artifact is an item that the project makes many times, such as a commit message or a module specification. A part is one piece of the artifact format, such as a heading, a labeled line, or a section.

## Template

A convention file has a title, a scope paragraph, and the sections below.

```markdown
# <topic> conventions

<scope of the convention>

## Template

<format of the artifact and the rules for each part, if any>

## Rules

<rules for the whole artifact, its file, and its life cycle>

## <special case>

<procedure or format for one situation, if any>

## Differences from the <skill name> skill

<skill rules that this convention replaces, if any>

## Reference

<tables that readers look up while they write, if any>

## Examples

<artifacts or parts of artifacts that show the convention, if any>
```

### Title

- Write `# <topic> conventions` in sentence case.
- Write the topic as the singular name of the artifact, such as "Commit message conventions". If the convention covers several artifacts, join their names with "and", such as "Markdown and English prose conventions".
- Keep the capitals of names and abbreviations, such as "GitHub Issue conventions" or "SQL file conventions".

### Scope

- Write one paragraph that starts with "This convention". State the artifact and what the convention controls, such as its format, name, or life cycle.
- If the artifact has a fixed location, name the location, such as `docs/adr/`.
- If the convention uses a term that a reader outside the project does not know, define the term in this paragraph.
- If another document owns the purpose of the artifact or the decision behind the convention, link to that document in this paragraph.
- Do not define terms in other sections.

### Template section

- If the artifact has no fixed format, omit this section.
- Start with one sentence that names the parts of the artifact. If a part is outside the body, such as the title of an Issue or a pull request, name the part in this sentence.
- After the sentence, put the format in a code block with the language of the artifact, such as `markdown` or `text`.
- In the code block, write fixed text as it appears in the artifact, such as `## Context`. Write each variable part as a placeholder in angle brackets.
- Show a part that repeats only once, such as one acceptance criterion. State how often it repeats in the subsection of the part.
- Write each placeholder as a lowercase noun phrase without a final period. State only what the part is, such as `<facts and limits that exist before the decision>`.
- Keep the capitals of names and abbreviations in a placeholder, such as `<module ID>` or `<Issue number>`.
- If a placeholder shows the format of a value, write the format in uppercase letters, such as `<NNNN>` for a four-digit number.
- For an optional part, end the placeholder with `, if any`. If the optional part has a heading, put `, if any` in the first placeholder under the heading.
- After the code block, write one `###` subsection for each part, in template order. Put the subsections for parts outside the body first, in the order of the first sentence.
- Name each subsection after its part. Use "Title" for the `#` heading line, the label for a labeled line such as `Date:`, and the heading text for a section such as `## Context`. For a heading that contains a placeholder, use its fixed words, such as "Differences from the skill". For other parts, use a short noun phrase from the placeholder.
- If the name of a subsection repeats another heading in the convention file, add a word that tells the headings apart, such as "Rules section" or "Result gaps".
- Write at least one rule in each subsection, such as the format, length, source, or links of the part.
- Write only bullets in each subsection. If a rule applies steps in order or by precedence, put a numbered list inside its bullet.
- If a part contains a table, add a `####` subsection for the table under the part. Write bullets for the whole table, and then a table with the columns `Column` and `How to write` that has one row for each column of the described table.
- Do not repeat the meaning from the placeholder in the subsection.

### Rules section

- Write only bullets in this section.
- Put the rules for the file, location, numbering, and life cycle of the artifact here. Also put the rules that cover more than one part here.
- Include one bullet that states how existing artifacts follow a change to the convention. Choose one of these approaches: update every existing artifact in the same PR, or apply the change only to new artifacts and to artifacts that another task changes.
- If the section has more than five bullets, group them by topic under `###` headings, such as "Files and numbering" or "Status and changes". If it has five bullets or fewer, do not add `###` headings.
- Do not put a rule about one part in this section. Put the rule in the subsection of that part.

### Special case

- Add one section for each situation that needs its own procedure or format, such as the [gap comment](github-issues.md#gap-comment) in the Issue convention or the checks before a review.
- Name the situation in the heading as a noun phrase.
- Start with paragraphs that state when the situation applies and the procedure to follow.
- If rules apply to the whole situation, write them as bullets after the paragraphs.
- If the situation has a fixed format, write the format after the bullets as the [Template section](#template-section) states. Use `###` subsections for the parts.
- If a situation contains a smaller situation, such as the result comment of the final Prove task, write the smaller situation as a `###` subsection with the same layout. Use `####` subsections for its parts.
- Do not put a situation inside a smaller situation.

### Differences from the skill

- If a skill in `.agents/skills/` covers the artifact and gives a different file path, format, section, or workflow, add this section.
- Use the folder name of the skill in the heading, such as "Differences from the spec-driven-development skill".
- Start with one sentence that links the `SKILL.md` file of the skill and states that this convention applies where the two differ.
- Write one bullet for each difference. Start with the project rule, and then state the skill rule that it replaces, such as "Save the specification as `docs/specs/<module-id>.md`. The skill saves it as `SPEC-<module-id>.md`."
- Link to a rule in this convention instead of repeating the rule.

### Reference

- Put each table under its own `###` heading, such as the [commit types](commit-messages.md#types).
- Write only tables in this section. If a table needs an introduction, write one sentence before the table that states what it lists.
- Link to the table from each rule that uses it.
- Do not put rules in this section. Put each rule in the subsection that uses the table.

### Examples

- Write one sentence before each example that states what the example shows.
- Show each example in a code block with the language of the artifact. If the examples are short names, show them in a table.
- If the examples show several kinds of artifacts, group them under `###` headings named after the kind, such as "Checkpoint commits".
- If an artifact is too long to show, link to it in the repository or on GitHub instead.
- Make each example follow every rule of the current convention, except a counter-example.
- If you add a counter-example, state in the sentence before it that it does not follow the convention. Name the rule that it breaks.
- Do not use bullets for examples.

## Rules

### Files

- Save the file as `docs/conventions/<name>.md`. Write the name as the topic in plural form, in lowercase words separated by hyphens, such as `commit-messages.md`. If the repository uses an abbreviation for the topic, use the abbreviation, such as `adrs.md`.
- Write one convention file for each kind of artifact. If a convention already covers the artifact, add the rule to that file.
- Put each rule in one convention file only. In other files, link to the rule instead of repeating it.

### Agent instructions

- Add a row for each convention file to the convention table in the [agent instructions](../../AGENTS.md#conventions).
- Put the row in the group of the work that first needs the convention. If the work happens in several groups, use the earliest group in the table.
- In the group, put the rows in the order in which the work happens.
- In the `Work` column, write a phrase that starts with a verb ending in "-ing" and names the artifact, such as "Writing or updating module plans in `tasks/<module-id>.md`". If the artifact has a fixed location, name the location.
- In the `Convention` column, link to the file. Use the topic in plural form as the link text, such as "Commit messages".

### Sections and wording

- Follow the [Markdown and English prose conventions](markdown-and-prose.md) in each convention file.
- Start each bullet with a verb, such as "Write", "Put", "Use", "Link", or "Omit". If the rule has a condition or applies to one place, state the condition or the place first, such as "In the index, use the record number". Apply this rule to every bullet outside code blocks.
- Give each bullet one main point, as the [Markdown rules](markdown-and-prose.md#markdown) define. Keep the condition, reason, and exception of the point in the same bullet.
- Order the bullets in each list: placement and format first, then the order and structure of the content, then allowed values and links, and then the rules that start with "Do not".
- Use the sections in the template order. Omit an optional section that has no content.
- Do not add other top-level sections. Add a new section to this convention before you use it in a convention file.

### Changes

- Apply a change of this convention only to new convention files and to convention files that another task changes. In the PR of that task, make the whole changed file follow this convention.
