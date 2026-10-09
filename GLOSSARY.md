# Glossary

This file defines each term that has a narrower or different meaning in Flowspace than in general English. The [Glossary entry](docs/conventions/glossary-entries.md) conventions state how to write an entry.

## Writing

| Term | Meaning |
| --- | --- |
| GitHub text | The text of Issues, pull requests, comments, and milestones on GitHub. |
| Markdown text | The text of Markdown files and GitHub text. |

## Conventions

| Term | Meaning |
| --- | --- |
| artifact | An item that the project makes many times, such as a commit message or a module specification. |
| convention file | A file in `docs/conventions/` that tells agents and developers how to write 1 kind of artifact. |
| part | 1 piece of the format of an artifact, such as a heading, a labeled line, or a section. |
| skill | A set of agent instructions in `.agents/skills/`. |

## Architecture decision records

| Term | Meaning |
| --- | --- |
| index | The table of records in the [Architecture Decision Records](docs/adr/README.md) file. |
| record | 1 architecture decision record (ADR). |

## Module specifications

| Term | Meaning |
| --- | --- |
| condition | A statement of when an effect happens, or why. |
| consumer | An API client, an event consumer, or a developer who reads logs, traces, metrics, dashboards, or alerts. |
| index | The [Specifications](docs/specs/README.md) table, which lists every module. |
| material risk | A failure that can break a success criterion or allow a threat in a threat model. |
| module | 1 capability that can be tested on its own. Its specification states what the capability does. |
| shape | A name, a route, a field, or a value that a consumer can read. |

## Module plans and Issues

| Term | Meaning |
| --- | --- |
| checkpoint | The list of the outcomes of a phase. After the tasks of the phase are done, a reviewer makes sure that each outcome holds. |
| final Prove task | The last task of a module plan, which proves the approved specification. |
| gap | An item in the acceptance criteria or the verification of an Issue that failed or did not run. |
| module plan | A plan that breaks an approved module specification into tasks. Each task becomes 1 GitHub Issue. |
| ordinary task | A task that is not the final Prove task. |
| task | 1 piece of work that 1 PR completes. |

## Code and tests

| Term | Meaning |
| --- | --- |
| capability | A task that a service performs. |
| request file | A `.bru` file in `tests/smoke/bruno/` that sends 1 HTTP request and tests the response. |
| request name | The `name` value in the `meta` block of a request file. |
| rule title | The `title` field of a provisioned Grafana alert rule. |
| run position | The `seq` value in the `meta` block of a request file. |
| scenario test | A test that checks 1 capability across several production files. |

## Commits and pull requests

| Term | Meaning |
| --- | --- |
| approval stamp | A file that records that a reviewer approved 1 exact version of a change, such as a staged tree or the text of a PR description. |
| area label | An `area:*` label that names a repository area. |
| author agent | The agent that writes a change. |
| change that breaks callers | A change that `buf breaking` reports, or that removes or changes the meaning of a field, a route, or an event that callers use. |
| checkpoint commit | A commit on a branch that records 1 tested work step. |
| lasting record | The What changed, Why, Breaking changes, and Related issues sections of a PR description. Each squash commit has only the PR title, so these sections hold the details of the change for a later reader of `main`, as [ADR-0043](docs/adr/0043-squash-commits-keep-only-the-pull-request-title.md) states. |
| main change | The change that a commit exists to make. Tests, generated output, and documents that change because of the main change are not part of it. |
| maintainer | The person who reviews and merges each PR. |
| material | Of a risk or an effect of a PR: able to change the decision to merge, or in need of an action after the merge. |
| ready | Of a PR: the local checks that the [Workflow](docs/conventions/pull-requests.md#workflow) rules of the pull request convention name and CI pass, and the title, description, and labels match the final work. |
| review comment | A comment or a review on a PR in GitHub. Feedback in the chat is not a review comment. |
| review role | A check of 1 aspect of a change, such as the code or the conventions, that a reviewer runs before a commit or a PR. The [Review roles](.agents/rules/review-roles.md) rules list the roles. |
| reviewer | An agent that runs 1 review role. |
| runner | A module in `.agents/scripts/review-runners/` that starts the CLI of 1 agent for a review. |
| service paths | The paths of a service, with the service name in place of `<service>`: `services/<service>/`, `contracts/proto/flowspace/<service>/`, `contracts/events/flowspace/<service>/`, `deploy/base/<service>/`, `deploy/overlays/local/<service>/`, `docs/specs/<service>-*`, `tasks/<service>-*`, `docs/<service>-*`, `docs/security/<service>-*`, `scripts/*<service>*`, the `<service>:*` tasks in `Taskfile.yaml`, and the Bruno requests in `tests/smoke/bruno/` that call the service. |
| staged tree | The Git tree object of the files that `git add` prepared for the next commit. |
| type label | A `type:*` label that names the type of a change. |
| work for later | Work that a PR does not do but shows to be needed, such as a gap that the PR finds or a step that its goal still needs. |

## Automatic builds

| Term | Meaning |
| --- | --- |
| agent comment | A comment that an agent posts on an Issue or a PR. |
| automatic build | A run of `/build` in `chain`, `fanout`, or `swarm` mode, in which a dispatcher agent starts subagents for ready Issues and for PRs that need work, as [ADR-0040](docs/adr/0040-automatic-builds-run-one-subagent-for-each-ready-issue.md) and [ADR-0042](docs/adr/0042-the-build-command-has-four-modes.md) state. |
| marker | An HTML comment that starts an agent comment and names its kind. Agents post with the account of the maintainer, so a marker tells an agent comment apart from a comment of the maintainer. |
| requested change | A maintainer feedback comment, as [ADR-0041](docs/adr/0041-maintainers-answer-automatic-builds-in-the-chat.md) states. A requested change is open until its `Done in` line names a commit that the PR branch contains. |
| restack | A move of the commits of a stacked PR onto the new state of its parent PR or of `main`. |
| stack line | The line of a stacked PR that names its parent PR, also after the parent PR merges. |
| stacked PR | A PR from an automatic build that started on the branch of another PR from an automatic build, its parent PR. |
