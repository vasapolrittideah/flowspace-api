# Module evidence conventions

This convention defines the file, the format, and the changes of the evidence file of 1 module in `docs/evidence/`.

[ADR-0051](../adr/0051-final-prove-task-evidence-lives-in-module-evidence-files.md) states why the evidence of the final Prove task lives in this file and not in the pull request (PR) description.

## Template

An evidence file has a title, a module ID, a proof sentence, [Coverage](#coverage), [Local checks](#local-checks), and [Gaps](#gaps).

```markdown
# Evidence: <capability name>

Module ID: `<module ID>`

This evidence proves the [<capability name>](../specs/<module-id>.md) specification in [PR #<PR number>](<PR URL>).

## Coverage

Tests prove these criteria and threat IDs:

| Criteria and threat IDs | Tests |
| --- | --- |
| <criteria and threat IDs> | <tests and the scope that they prove> |

## Local checks

<stack sentence, if any>

| Check | Result |
| --- | --- |
| <check> | <result> |

## Gaps

- <partial proof or gap>
```

### Title

- Copy the capability name from the title of the approved specification, without the `Spec:` prefix.

### Module ID

- Copy the module ID from the approved specification, in backticks.

### Proof sentence

- Link the capability name to the specification file.
- Link each PR with its full URL, and use `PR #<number>` as the link text.
- Name the PR of the final Prove task first. Then name each PR that resolved a gap or completed a partial proof, in the order of their merges, such as "in PR #541 and PR #560", with a link on each PR.
- Until the PR of the final Prove task opens, end the sentence after "specification", because the PR has no number yet. The [Workflow](#workflow) rules state when to add the number.

### Coverage

- Start the section with the fixed sentence "Tests prove these criteria and threat IDs:".
- Write the tests as the [Coverage table](#coverage-table).

#### Coverage table

- Write 1 row for each set of tests. If 1 set of tests proves several criteria or threat IDs, put them in 1 row.
- Order the rows by the first criterion of each row. Put the rows that name only threat IDs last.
- Write the columns in this order.

| Column | How to write |
| --- | --- |
| `Criteria and threat IDs` | The success criteria and the threat IDs of the row, as the [Format and content](#format-and-content) rules state, such as "Criteria 4 and 6, `ID-T01`, and `ID-T02`". |
| `Tests` | The name of each test function or subtest in backticks, followed by "for" and the scope that it proves, such as "`TestReauthenticationTelemetryOmitsSecrets` for logs, spans, metric attributes, and error responses". Write a subtest as `<test function>/<subtest name>`. |

### Local checks

- Write the stack sentence and the [Local checks table](#local-checks-table).
- In the stack sentence, write "The checks ran against \<local stack\>." Name the local cluster or environment and each service that a check ran against, such as "k3d `flowspace-local` with Mailpit".
- If no local check applies and no Issue item failed or did not run, write the fixed sentence "No local check applies outside CI." instead of the stack sentence and the table.

#### Local checks table

- Write 1 row for each local check that CI does not run, such as `task smoke:bruno`.
- Write 1 row for each Issue item that failed or did not run, also when the item is a test or a check that [CI](../../.github/workflows/ci.yml) runs.
- Do not include the other checks that CI runs, such as `task check:task` or the Buf commands.
- Write the columns in this order.

| Column | How to write |
| --- | --- |
| `Check` | The command in backticks, such as `` `task smoke:bruno` ``. If the check has no command, write a short action, such as "GitHub provider login with a real GitHub account". |
| `Result` | `Passed`, `Failed`, or `Not run`. Then, when the check proves success criteria or threat IDs, name them as the [Format and content](#format-and-content) rules state. Then write the passed and total counts when the check reports them, such as "Passed 44/44 requests". For a result from another PR, write "in PR #\<number\> on \<YYYY-MM-DD\>" after the criteria and counts. Link the PR with its full URL. After a colon, state what the check showed, such as "Passed criteria 6 and 7 in PR #470 on 2026-10-12: the outbox age rose to 102 s and returned to 0". |

### Gaps

- Write 1 bullet for each partial proof and each gap. Put the partial proofs first.
- For a partial proof, name the criteria or threat IDs, state the part that the tests do not prove, and state the reason that the module can still close.
- For a gap, name the Issue item and its result, `Failed` or `Not run`, and state the reason.
- If the file has no partial proof and no gap, write the fixed sentence "None." instead of the bullets.

## Rules

### Naming and location

- Save 1 evidence file as `docs/evidence/<module-id>.md`, with the module ID of its specification.

### Format and content

- Use the sections in the template order.
- Make sure that each success criterion of the specification, and each threat ID in its [Testing strategy](module-specs.md#testing-strategy), appears in the Coverage table or in the Result column of the Local checks table.
- Write a success criterion as its number without backticks, such as "Criterion 2" at the start of a cell and "criterion 2" after the start. Do not add IDs, such as `SC-01`, as the [Success criteria](module-specs.md#success-criteria) rules state.
- Write a threat ID as the [Identity threat model](../security/identity-threat-model.md) writes it, with 2 digits, in backticks, such as `ID-T03`.
- Write the criteria first and then the threat IDs, each in ascending order.
- Write all criteria of a cell as 1 item, such as "Criteria 2, 4, and 5", and each threat ID as 1 more item, such as "Criteria 4 and 6, `ID-T01`, and `ID-T02`".
- Separate 3 or more items with commas, and put "and" before the last item. Separate 2 items with "and" only.
- In every section, name a real account only by its type, such as "a real GitHub account". Do not write the account name, a subject ID, a token, or another secret.
- Do not add other top-level sections. Add a new section to this convention before you use it in an evidence file.

### Workflow

- Add the evidence file in the PR of the final Prove task, as the [Final Prove task PR](pull-requests.md#final-prove-task-pr) rules state.
- After the PR opens, add its number to the proof sentence in a separate commit.

### Changes

- When a later PR proves a changed specification again, rewrite the whole file.
- When a PR resolves a gap or completes a partial proof, update only its rows, remove its bullet from the Gaps section, and add the PR to the proof sentence. If the resolved item is a test that CI runs, replace its Local checks row with a Coverage row, as the [Coverage table](#coverage-table) rules state.
- Between these changes, change the file only to fix a wrong fact.
- Apply a change of this convention only to new evidence files and to evidence files that a later PR changes. A module that closed before ADR-0051 has no evidence file.

## Examples

This Coverage table follows the convention:

```markdown
| Criteria and threat IDs | Tests |
| --- | --- |
| Criteria 2, 4, and 5 | `TestIdentityRepository/re-authentication through the public routes` for a password proof that opens a closed window and a provider proof that keeps its end time |
| Criterion 17 and `ID-T17` | `TestReauthenticationTelemetryOmitsSecrets` for logs, spans, metric attributes, and error responses |
```

These Local checks rows follow the convention:

```markdown
| Check | Result |
| --- | --- |
| `task smoke:bruno` | Passed 56/56 requests |
| GitHub provider re-authentication with a real GitHub account | Passed criterion 10: the callback page asked for a re-authentication code, and the window of the starting session opened |
```
