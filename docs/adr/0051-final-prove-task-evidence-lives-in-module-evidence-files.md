# ADR-0051: Final Prove task evidence lives in module evidence files

Date: 2026-10-22

Status: Accepted; scoped exception to [ADR-0043](0043-squash-commits-keep-only-the-pull-request-title.md)

## Context

The final Prove task of a module plan proves the approved specification with tests and local checks. A reader of a later version of `main` needs to know which test proves each success criterion and each threat ID, and which local checks ran. The review roles need the same facts before the pull request (PR) opens.

[ADR-0043](0043-squash-commits-keep-only-the-pull-request-title.md) makes the PR description the lasting record of each change. The [Final Prove task PR](../conventions/pull-requests.md#final-prove-task-pr) rules put the evidence at the end of the What changed section, as a stack sentence and a table of local checks. The review roles run in a read-only sandbox that cannot read GitHub, so the author agent copies the description into an input file for each review round. After the merge, an agent edits the description only to fix a wrong fact, so a later proof of the same specification goes into a new PR. A check that ran in an earlier PR stays in that PR, such as the broker outage check that PR #472 cites from PR #470.

The rules do not say which test proves each criterion, how to write criterion numbers and threat IDs, or where a partial proof goes. PR #472 and PR #541 use different tables for the same kind of evidence.

## Decision

The evidence of each final Prove task lives in `docs/evidence/<module-id>.md`, 1 file for each module. This is a scoped exception to ADR-0043: the PR description stays the lasting record of the change, and the evidence file is the lasting record of the proof.

An evidence file has 3 sections. Coverage maps each success criterion and threat ID to the tests that prove it. Local checks names the local stack and each check that CI does not run, with its result. Gaps states each partial proof, with the reason that the module can still close, and each gap. Every success criterion and every threat ID of the specification appears in Coverage or in Local checks. For a check that ran with a real account, the file names only the type of the account and the result, never the account, a subject ID, or a token.

The PR of the final Prove task adds the evidence file. After the PR opens, a separate commit adds the PR number to the file. When a later PR proves a changed specification again, it rewrites the whole file, and git keeps the earlier proof. The PR that resolves a gap or completes a partial proof updates only its rows and removes it from Gaps. Otherwise, between proofs, a PR changes the file only to fix a wrong fact.

The What changed section of the PR describes the status changes and the added tests, and it links to the evidence file. A partial proof goes only in the Gaps section of the file, not in the Risks or limitations section of the PR. When an Issue item fails or does not run, the Local checks section of the file lists its check with the result `Failed` or `Not run`. This holds also when the item is a test or a check that CI runs. The Gaps section names the item too. The PR also states it in Risks or limitations and refers to the Issue with `Refs`, as the Final Prove task PR rules state.

This record applies to each final Prove task PR that is open or that opens after this record, starting with PR #541. Modules that closed before this record keep their evidence in their PR descriptions. This record adds no link from a specification to its evidence file.

## Alternatives Considered

### Stricter evidence rules in the PR description

- Pros: no new kind of file, and each PR holds all of its evidence.
- Cons: the review roles still need a copy of each description, and evidence from several PRs stays in several places.
- Rejected: a file in the repository lets each review role read the evidence directly and keeps the proof of a module in 1 place.

### An evidence section in the specification

- Pros: each criterion and its proof are in the same file.
- Cons: each proof changes an approved specification, and test results that change with each run mix with the contract.
- Rejected: a separate file keeps the specification as the approved contract and lets the proof change without an approval.

### A comment on the Issue of the final Prove task

- Pros: GitHub shows the evidence next to the task.
- Cons: the review roles cannot read GitHub, and each later proof has a new Issue, so the evidence of a module spreads across Issues.
- Rejected: a file in the repository is readable in the sandbox and keeps the evidence of a module in 1 place.

### A new section for each proof in the same file

- Pros: 1 file shows every proof of the module.
- Cons: an old result stays next to the current specification, and a reader can take it as current.
- Rejected: git keeps each earlier proof, so a rewrite shows only the evidence for the current specification.

## Consequences

- The review roles read the evidence in the checkout, and the input file of a review holds only the PR metadata and the Issue.
- Each final Prove task PR has 1 more file and 1 more commit for the PR number. That commit needs a new code-reviewer approval, and its push needs the convention-reviewer and writing-reviewer approvals again.
- A rewrite removes the earlier evidence from the current file, so a reader uses `git log` to find an earlier proof.
- An evidence file records the results of 1 version of the code. A later change can make a result old, so the PR number in the file shows which proof the results come from.
- The evidence of modules that closed before this record stays in their PR descriptions, so evidence is in 2 kinds of places until each of these modules is proved again.
- PR #541 waits for the conventions of this record before it can merge.
- A later PR must add the Module evidence conventions and change the Pull requests conventions, the GitHub Issues conventions, the Repository instructions in `AGENTS.md` and `CLAUDE.md`, and the Project structure.
