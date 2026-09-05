---
name: create-adr
description: >
  Decide whether something warrants an architecture decision record in this
  repository, and draft it under docs/adr/ if it does. Use when the user asks
  to write, create, record or draft an ADR or an architecture decision; when
  asking whether a choice needs one; when a change requires a decision to land
  alongside it; or when a new decision may replace or contradict an existing
  ADR and needs to supersede it. May conclude that no ADR is warranted, or
  that an accepted one already covers the question.
allowed-tools: Read, Write, Edit, Glob, Grep, Bash(ls:*), Bash(npx markdownlint-cli2:*)
---


Draft an ADR for the topic the user named.

1. **Check it is a decision, and a new one.** Read `docs/adr/` first. If an
   accepted ADR already covers this, say so and stop. If this contradicts one,
   it is a supersession: write the new ADR and set the old one's Status to
   `Superseded by ADR-NNNN`, which ADR-0001 permits because Status is metadata
   rather than a decision. If reversing the choice would touch one service and
   no data, it is not architecturally significant and gets no ADR.

2. **Number, name, and start from the template.** The next number is the
   highest existing plus one, never reused. The filename slug carries no
   articles — `0009-one-go-module-for-repository.md`. Copy
   `docs/adr/template.md` and keep its section order exactly.

3. **Gather the forces before writing anything.** Read the constraints in
   `CLAUDE.md` and every ADR this one builds on. Ask the user for whatever the
   repository cannot answer — a threshold, a product requirement, a tolerance
   for loss. An ADR written from assumptions documents the assumptions.

4. **Status is `Accepted` only if the decision has actually been made.** If any
   part is still your suggestion rather than the user's choice, the status is
   `Proposed` and the open question is written into the Decision, not hidden.

5. **Context states forces, not conclusions.** Facts, constraints, and what
   would surprise a reader in six months. If the Context already argues for the
   answer, it is a sales pitch. Say what deferring the decision would cost.

6. **The Decision must be testable.** A reader has to be able to tell whether a
   given pull request violates it. "Prefer", "consider" and "where appropriate"
   are not decisions. Operational rules that make the decision safe belong in
   the Decision, explicitly marked as such, not left as implementation detail.

7. **Cite by number only ADRs whose files exist.** Name a decision not yet
   taken by its subject — "the migration tool" — never by a number reserved in
   advance and never by the product you expect to win, which is the same drift
   in a different disguise. References point backwards only: never edit an
   older ADR so that it points forward at this one.

8. **Consequences have to cost something.** An empty "Negative" section means
   the analysis is unfinished, not that the decision is clean. Name the failure
   mode this decision creates. Every decision creates one.

9. **Alternatives get a real reason.** Reject on a stated axis — footprint,
   operational cost, a guarantee the option cannot give. Rejecting a genuinely
   good option on a fair axis is stronger than beating a strawman, and it is
   the section a reader comes back for.

10. **"Revisit when" must be observable.** A metric, a threshold, a product
    event. "When needed" is not a signal. "Never" is a valid answer and is
    written down rather than left blank.

11. Align table pipes so `MD060` passes, wrap prose at 80 columns, and run
    `npx markdownlint-cli2 "**/*.md"` before reporting done.

Write the file only. It lands in the pull request carrying the first change
that depends on it (ADR-0001), so do not commit and do not open one here.
