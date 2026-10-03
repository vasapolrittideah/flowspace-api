---
description: Start spec-driven development — write a structured specification before writing code
---

Invoke the agent-skills:spec-driven-development skill. Use the skill for its clarification and review gates. Follow the [module specification conventions](../../docs/conventions/module-specs.md) for the format, file paths, modules, status, and approval. Where the skill differs, follow the [differences that the convention lists](../../docs/conventions/module-specs.md#differences-from-the-spec-driven-development-skill).

Before you ask questions, read these documents:

1. Read the module specification conventions and the [specification index](../../docs/specs/README.md). The index lists every module, including `Planned` modules that have no specification file yet.
2. Read the [constraints](../../CONSTRAINTS.md), the [architecture](../../docs/architecture.md), the [technology stack](../../docs/technology-stack.md), and the [ADRs](../../docs/adr/README.md) that apply to the capability. Treat an open proposal as undecided.
3. For an Identity capability, read the [Identity threat model](../../docs/security/identity-threat-model.md).

Use the workflow rules of the convention to decide whether the request changes an existing module or adds new modules.

Ask clarifying questions until you can write each section of the convention template without a guess. Cover these topics:

1. The objective, the first users, and their data.
2. The scope, the excluded work, and the ADRs that apply.
3. The module dependencies for the `Depends on` column of the index.
4. The contract that consumers see.
5. The behavior, including security, data compatibility, and diagnostics.
6. The material risks and the test level that proves each risk.
7. The implementation boundaries and the success criteria.

If the capability needs a decision that no accepted ADR makes, stop and ask the maintainer.

If the request bundles several independently testable capabilities, use these steps before you write a specification:

1. Propose a capability map in chat. A capability map lists the module IDs, the dependencies of each module, and the build order.
2. After the maintainer agrees, deliver the `Planned` rows of the map in their own PR, as the workflow rules of the convention state.
3. Write the specifications in the build order. Start each specification only when the workflow rules allow it.

Deliver one specification in each PR:

1. Save the specification as `docs/specs/<module-id>.md` with the `Draft` status. In the same commit, add or update its row in the specification index.
2. Before you commit, show the specification to the maintainer in chat and wait for their agreement. This agreement does not approve the specification.
3. Create the branch, commits, and PR as the delivery conventions in the [agent instructions](../../AGENTS.md) state. Use the approval PR title from the status rules of the convention. Stop after you open the PR. Do not plan or implement the module.
4. When the maintainer asks for approval, change the status to `Approved` in the specification and in the index in one commit, and push it to the same PR.

To change an existing specification, follow the status rules of the convention.
