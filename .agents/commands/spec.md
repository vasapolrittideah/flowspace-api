---
description: Start spec-driven development — write a structured specification before writing code
---

Invoke the agent-skills:spec-driven-development skill. Use the skill for its clarification and review gates. Follow the [module specification conventions](../../docs/conventions/module-specs.md) for the format, the status, and the approval, and the [differences that the convention lists](../../docs/conventions/module-specs.md#differences-from-the-spec-driven-development-skill) where the skill differs.

The work starts from a request to add a capability or to change one. It ends when the specification PR has the `Approved` status, CI passes, and the maintainer has the squash message. The maintainer merges the PR. Do not plan or implement the module.

## Prepare

1. Read the module specification conventions and the [specification index](../../docs/specs/README.md).
2. Read the [constraints](../../CONSTRAINTS.md), the [architecture](../../docs/architecture.md), the [technology stack](../../docs/technology-stack.md), and the [ADRs](../../docs/adr/README.md) that apply to the capability. Treat an open proposal as undecided.
3. For an Identity capability, read the [Identity threat model](../../docs/security/identity-threat-model.md).
4. Use the [workflow rules](../../docs/conventions/module-specs.md#workflow) of the convention to decide whether the request changes an existing module or adds modules.

If the request bundles several capabilities that can be tested on their own, propose a capability map in the chat first. A capability map lists the module IDs, the dependencies of each module, and the build order. After the maintainer agrees, deliver the `Planned` rows in their own PR, as the workflow rules state. Then write one specification for each PR, in the build order.

## Clarify

Ask questions until you can write each section of the [template](../../docs/conventions/module-specs.md#template) without a guess. If the capability needs a decision that no accepted ADR makes, stop and ask the maintainer.

## Write

1. Create a branch from `main`, as the [branch name conventions](../../docs/conventions/branch-names.md) state.
2. Write `docs/specs/<module-id>.md` and its row in the index with the `Draft` status. To change an existing specification, follow the [status rules](../../docs/conventions/module-specs.md#status-and-approval) to decide the status.
3. Show the specification to the maintainer in the chat, and wait for their agreement. This agreement does not approve the specification.

## Open the PR

1. Commit the specification and the index row together, as the [commit message conventions](../../docs/conventions/commit-messages.md) state.
2. Push the branch and open the PR, as the [pull request conventions](../../docs/conventions/pull-requests.md) and the [label conventions](../../docs/conventions/github-labels.md) state. Use the PR title from the status rules.
3. Wait for CI. The `Lint Markdown` job fails while the status is `Draft`, as the status rules state. When every other check passes, tell the maintainer that the PR is ready for review.

## Approve

1. When the maintainer asks for approval, change the status to `Approved` in the specification and in the index in one commit, and push it to the same PR.
2. Wait until every CI check passes.
3. Give the squash message in the chat, and tell the maintainer that the PR is ready. Stop.
