# Module specification conventions

Save each module specification as `docs/specs/<module-id>.md` and add it to the [specification index](../specs/README.md). Define one capability per specification before implementation.

Start the specification with its name, module ID, and current status:

```text
# Spec: <capability name>

Module id: `<module-id>`

Status: Draft.
```

Use `Draft` before approval, `Approved` after approval, and `Implemented` after every success criterion has evidence. An approved specification permits planning.

## Specification format

All capability specifications use the core sections through `Success criteria` in this order. Add a final section for open decisions or real-user readiness when needed. Use subsections under `Required behavior` for topics that are specific to one capability.

| Section | Content |
| --- | --- |
| `Objective` | Capability, users, purpose, and success intent |
| `Scope and decision sources` | Included and excluded work, module dependencies, and relevant decisions |
| `Contract` | Consumer-visible interfaces, inputs, outputs, data, and errors |
| `Required behavior` | Business rules, invariants, security, consistency, and operational behavior |
| `Commands` | Commands that build, test, lint, or generate artifacts for this capability |
| `Testing strategy` | Feature-specific evidence and its owning test boundaries |
| `Boundaries` | Capability-specific actions to always do, ask about, or never do |
| `Success criteria` | Observable Given and Then outcomes required for completion |
| `Open questions and approval` (optional) | Unresolved decisions and the approval required for the next phase |
| `Before real users join` (optional) | Checks that must pass before real-user use |

Use the [shared project sources](../specs/README.md#shared-project-sources) for technology stack and code style. Put implementation locations in the plan. Add a new top-level section to this convention before using it in a specification.

Repeat a shared rule only when it changes observable behavior or completion evidence. Put project-wide rules in their source documents instead of copying them into each specification. In Identity specifications, reference the applicable [threat IDs](../security/identity-threat-model.md) in each success criterion and abuse test.
