# Bruno request file conventions

This convention names Bruno request files in `tests/smoke/bruno/`. A request file is a `.bru` file that sends one HTTP request and tests the response. The [project structure](../project-structure.md) defines the purpose of the collection.

## Template

```text
<NN>-<verb-phrase>.bru
```

| Part | Content and format |
| --- | --- |
| `NN` | The `seq` value in the `meta` block, written with two digits. |
| Verb phrase | The `name` value in the `meta` block, in lowercase words separated by hyphens. |

## Rules

- Name each request with a verb phrase that states its action and the expected result. A request file is one step in an ordered run, so its name describes what the step does. Go component files use noun phrases because one file groups several actions of a capability.
- Write the `name` in the `meta` block in sentence case, and start it with a verb. Make the verb phrase in the file name match this `name`.
- If the API must refuse the request, start the `name` with `Reject`.
- Make `NN` equal the `seq` value. If you insert or move a request, update the `seq` value and the file number of each request that changes position.
- Apply this convention to new request files and request files changed for another task. Do not rename untouched files only to satisfy this convention. A naming cleanup needs its own reviewable change.

## Examples

| `name` in `meta` | File name |
| --- | --- |
| `Sign up` | `01-sign-up.bru` |
| `Reject duplicate signup` | `02-reject-duplicate-signup.bru` |
| `Read verification message` | `03-read-verification-message.bru` |
| `Log in with password` | `13-log-in-with-password.bru` |

The noun phrase `02-duplicate-signup.bru` does not follow this convention. It does not tell the reader whether the API must accept or refuse the request.
