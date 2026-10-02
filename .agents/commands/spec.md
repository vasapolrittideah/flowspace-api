---
description: Start spec-driven development — write a structured specification before writing code
---

Invoke the agent-skills:spec-driven-development skill. Use the skill for its clarification and review gates. Use the repository documents for the format, file paths, status, and delivery. If the skill gives a different rule, follow the repository.

Before you ask questions, read these documents:

1. Read the [module specification conventions](../../docs/conventions/module-specs.md) and the [specification index](../../docs/specs/README.md). The index lists every module, including `Planned` modules that have no specification file yet.
2. Read the [constraints](../../CONSTRAINTS.md), the [architecture](../../docs/architecture.md), the [technology stack](../../docs/technology-stack.md), and the [ADRs](../../docs/adr/README.md) that apply to the capability. Treat an open proposal as undecided.
3. For an Identity capability, read the [Identity threat model](../../docs/security/identity-threat-model.md).

Ask clarifying questions until you can write each section of the convention template without a guess. Cover these topics:

1. The objective, the first users, and their data.
2. The scope, the excluded work, the module dependencies, and the ADRs that apply.
3. The contract that consumers and log readers see.
4. The behavior, including security, data compatibility, and diagnostics.
5. The material risks and the test level that proves each risk.
6. The implementation boundaries and the success criteria.

If the capability needs a decision that no accepted ADR makes, stop and ask the maintainer.

Write the specification with the sections and the order of the convention template. Do not add the commands, tech stack, project structure, or code style sections of the skill. Give each module an ID in the `<service-or-area>-<capability>` format of the convention. The module IDs in the skill examples, such as `identity`, do not follow this format.

If the request bundles several independently testable capabilities, use these steps before you write a specification:

1. Propose a capability map in chat. A capability map lists the module IDs, the dependencies of each module, and the build order.
2. After the maintainer agrees, add one `Planned` row for each module to the specification index. Name the dependencies of each row. Do not create a separate map file.
3. Deliver the index change in its own PR. Do not write a specification for a module in the map until this PR merges.
4. Write the specifications in the build order.

Deliver one specification in each PR:

1. Save the specification as `docs/specs/<module-id>.md` with the `Draft` status.
2. In the same commit, add or update its row in the specification index with a link and the `Draft` status.
3. Before you commit, show the specification to the maintainer in chat and wait for their agreement. This agreement does not approve the specification.
4. Create the branch, commits, and PR as the delivery conventions in the [agent instructions](../../AGENTS.md) state. Stop after you open the PR. Do not plan or implement the module.

Only the maintainer approves a specification. The approval is the merge of a separate PR that changes the status to `Approved`, as the status rules of the convention state. Change the status to `Approved` only when the maintainer asks for it.

If you change an `Approved` or `Implemented` specification, follow the status rules of the convention. A change to the contract, behavior, or success criteria sets the status to `Draft` again in the specification and in the index.
