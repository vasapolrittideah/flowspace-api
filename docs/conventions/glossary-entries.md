# Glossary entry conventions

This convention defines the format and the place of each entry in [`GLOSSARY.md`](../../GLOSSARY.md) at the root of the repository.

## Template

An entry has a term and a meaning, in a table row under a topic heading.

```markdown
## <topic>

| Term | Meaning |
| --- | --- |
| <term> | <meaning in the project> |
```

### Term

- Write the term as the project text uses it, in lowercase except for names and abbreviations, such as "task", "PR", or "GitHub text".
- Write a noun or a noun phrase in the singular form. If the term names a set, such as "service paths", write the plural form.
- If a term describes a quality, such as "ready", write the adjective, and start the meaning with the noun that it describes, such as "Of a PR:".

### Meaning

- Write 1 or more sentences that state what the term means in the project.
- If a document owns the decision behind the term, such as an ADR, link to it.
- Do not write rules in the meaning. Put each rule in the convention or the document that owns it.

## Rules

### Format and content

- Write the topics as `##` headings in the order of the work, from writing and conventions to delivery and automatic builds.
- Put each entry in the topic of the work that first needs the term.
- In each topic, order the entries by term in alphabetical order.
- If 2 topics use the same term with different meanings, add an entry under each topic.
- Define only a term that has a narrower or different meaning in the project than in general English.
- Do not define a term in another file, such as a convention file, a specification, or an ADR.

### Workflow

- If a change uses a new term, or changes the meaning of a term, add or update its entry in the same PR.

### Changes

- If a term is no longer used, remove its entry in the PR that removes its last use.
- Apply a change of this convention only to new entries and to entries that a later PR changes.

## Examples

This entry defines a term with a project meaning:

```markdown
| task | 1 piece of work that 1 PR completes. |
```

This entry describes a quality of a PR:

```markdown
| ready | Of a PR: the local checks that the Workflow rules of the pull request convention name and CI pass, and the title, description, and labels match the final work. |
```
