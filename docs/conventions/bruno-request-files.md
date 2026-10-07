# Bruno request file conventions

This convention defines the file name and the request name of each Bruno request file in `tests/smoke/bruno/`. A request file is a `.bru` file that sends one HTTP request and tests the response. The request name is the `name` value in the `meta` block of the file. The run position is the `seq` value in the same block. The [Project structure](../project-structure.md) defines the purpose of the collection.

## Template

A file name has a run position and a request name.

```text
<run position>-<request name>.bru
```

### Run position

- Write the run position with two digits, such as `01` for `seq: 1`.

### Request name

- In the file name, write the request name in lowercase words separated by hyphens, such as `log-in-with-password` for `Log in with password`. Keep each hyphen that the request name contains, such as in `all-session`.
- In the `meta` block, write the request name as a verb phrase in sentence case. A request file is one step in an ordered run, so its name describes what the step does. Go component files use noun phrases because one file groups several actions of a capability.
- If the API must refuse the request, start the request name with `Reject`.

## Rules

- Number the requests from 1 in the order of the run. If you insert or move a request, update the run position and the file name of each request that changes position.
- Apply a change of this convention to new request files and to request files that a later PR changes. Rename an unchanged request file only in a PR that only renames request files.

## Examples

These request names and file names follow the convention:

| Request name | File name |
| --- | --- |
| `Sign up` | `01-sign-up.bru` |
| `Reject duplicate signup` | `02-reject-duplicate-signup.bru` |
| `Log in with password` | `13-log-in-with-password.bru` |
| `Reject current session after all-session logout` | `23-reject-current-session-after-all-session-logout.bru` |
| `Reject GitHub state on the Google callback` | `40-reject-github-state-on-the-google-callback.bru` |

This request name does not follow the convention, because it is a noun phrase. It does not tell the reader whether the API must accept or refuse the request:

| Request name | File name |
| --- | --- |
| `Duplicate signup` | `02-duplicate-signup.bru` |
