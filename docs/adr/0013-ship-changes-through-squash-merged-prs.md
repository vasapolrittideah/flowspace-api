# ADR-0013: Ship every change through a squash-merged pull request

- **Status:** Accepted
- **Date:** 2026-09-05
- **Deciders:** Vasapol Rittideah
- **Tags:** `delivery`

## Context

Earlier decisions settled what is deployed (ADR-0011) and how it is developed
(ADR-0012). What is still open is the path between them: how a change gets from
a branch onto a server, what stops it, and what it leaves behind in history.

Two facts make this more than a matter of taste.

**Rollback is a git operation.** ADR-0011 made deployment state a commit, so
rolling back means reverting one. That only works if the thing to revert is a
single addressable commit rather than a range somebody has to reconstruct
during an incident.

**There is one maintainer.** A rule that says "requires an approving review" is
a checkbox that always passes. Any gate that depends on a second person is
theatre, so every gate here has to be something a machine can fail.

There is also an existing complication: CI already writes to main, because
image digests are committed there. main is not a branch only humans touch.

## Decision

### Trunk-based, and always through a pull request

Branches are short — a day or two — and merge into main, which stays
deployable. Direct pushes to main are denied to people. The one exception
belongs to the CI identity that updates image digests, and that is the shape
exceptions must take: **granted to an identity, never available to a judgement
call.**

A one-line change goes through a pull request like anything else. Not because
it deserves scrutiny, but because a control that can be waived when something
feels small is not a control. The pull request is also what creates the preview
environment, so it is a mechanism rather than a ritual. Chores that feel too
small to be worth it are batched into a pull request that is open anyway.

### Squash merge, one commit per pull request

Every commit on main is a state that CI proved green. That is the requirement
`git bisect` has, and rebase-merging breaks it: CI runs on the pull request
head, never on each intermediate commit, so `wip` and `fix typo` would land on
main having never been built. A bisect that stops on a commit that does not
compile is a bisect that answers nothing.

Commits on a branch are free-form and are discarded at merge. There is no
commitlint, no hook, and no format requirement on them. They exist so work can
be committed every few minutes without thought, and a format rule on a message
that is about to be deleted only discourages the habit.

Repository settings are part of this decision, not configuration around it:

- Squash is the only merge button. Merge commits and rebase merging are off.
- The squash message is **pull request title and description**. The option
  named "default message" must not be used: on a single-commit pull request it
  takes the branch commit message instead, and the convention below breaks
  silently, with no error anywhere.

### Conventional Commits, on the pull request title only

The scope is the service directory the change touches, or `repo` for
repository-wide changes. A breaking contract change carries `!`:

```text
feat(workitem)!: replace status enum with lifecycle states
```

That mark lines up with what already blocks the merge in CI — `buf breaking`
for events, `oasdiff` for the HTTP API — and with the ADR explaining it, which
lands in the same pull request under ADR-0001.

CI checks the title with a regular expression. Nothing else is checked, and
nothing automated consumes the convention today: artifacts are addressed by
digest (ADR-0011), so there is no semantic version for it to drive. This is
recorded so that nobody later installs release automation looking for a job.

### Machines are the only gate

A pull request merges when, and only when:

- CI is green — linters, tests, `buf lint`, `buf breaking`, `oasdiff`,
  generated code matching what is committed, and markdownlint.
- Its preview environment deploys and the Newman smoke collection from
  ADR-0010 passes against it.

There is no approval requirement. ADR-0001's rule that an ADR lands with the
first change depending on it is still checked by a person, and is the first
thing worth automating.

There is also no pull request template. A template fills the body, the body is
the commit message, and so a template writes headings and checkboxes into
`git log` permanently — including ones hidden in HTML comments, which are text
like any other. The checks a template would list are executed instead: the
command that opens a pull request reads the diff, derives the scope, asks
whether a contract change is breaking, and flags a decision that ADR-0001 says
should be landing alongside. A check that runs is worth more than a box that is
ticked by the person it was meant to check.

### Merging deploys staging; production is a deliberate act

A merge to main is synced to staging automatically. Production is promoted by
moving the digest, by a human, on purpose.

That changes when **all five** of the following hold — not three of them:

1. Argo Rollouts canary with automated analysis is running in production.
2. The smoke collection runs against the canary and can fail the rollout.
3. Automated rollback has been proven by deliberately deploying a broken build
   and watching it revert with nobody touching it.
4. An SLO with multi-window burn-rate alerting exists, to catch the slow
   degradation a smoke test cannot see.
5. Mean time to rollback is measured and under five minutes.

Criterion three is the one that gets skipped, for the same reason ADR-0006
insists that compensations be exercised in CI: a recovery path that has never
run is not known to work.

## Consequences

### Positive

- Every commit on main is deployable, bisectable and revertable on its own.
- The pull request is the unit of everything — review, preview environment,
  smoke run, history entry, and rollback address.
- The gates can actually fail, which is more than an approval rule could say.
- Branch history needs no tending. There is no reason to rewrite it before
  merging, because it is discarded either way.

### Negative / accepted costs

- A one-line change costs a pull request and roughly a minute. This is accepted
  deliberately, and batching is the response rather than an exception.
- The step-by-step history of a change is not on main. A change whose
  intermediate steps genuinely matter should have been several pull requests.
- **main mixes squashed human commits with machine commits that move image
  digests.** Reverting code does not roll back a deployment, and reverting a
  digest does not undo code. Whoever is reverting has to know which of the two
  they are looking at, and this is the sharpest edge in this decision.
- CI is the only gate, so a blind spot in CI is a blind spot in the process
  with nothing standing behind it.
- Production recovery is only as fast as somebody being awake, until the five
  criteria above are met.

### Neutral

- Nothing here assumes a single maintainer except the absence of an approval
  rule. Adding reviewers later is a branch protection setting, not a redesign.

## Alternatives considered

| Option                                     | Why not                                                                                                                                        |
| ------------------------------------------ | ---------------------------------------------------------------------------------------------------------------------------------------------- |
| Pushing trivial changes straight to main   | An exception invoked by judgement is not a control. Allowing it means branch protection does not exist, including for the changes that need it |
| Merge commits                              | Rollback needs a different incantation, and main carries commits that CI never built                                                           |
| Rebase and fast-forward                    | Linear history, but every intermediate commit lands on main untested, which is exactly what bisect cannot tolerate                             |
| GitFlow with develop and release branches  | Solves parallel maintained releases and staged hardening, neither of which exists here                                                         |
| Requiring an approving review              | With one maintainer it is a checkbox that always passes, which is worse than no rule because it looks like one                                 |
| Conventional Commits on branch commits too | Enforces a format on messages that are deleted at merge, and discourages committing often, which is the one thing branch commits are for       |
| Continuous deployment to production now    | The automated rollback path has never been executed. Shipping without it means the first real test is an incident                              |

## Revisit when

- All five continuous deployment criteria hold, which turns production
  promotion from a decision into a pipeline stage.
- A second person commits to this repository. Approval stops being theatre at
  that moment and branch protection should change the same day.
- Branches routinely live longer than two days, which means pull requests have
  grown past what this model supports.
- Someone reverts the wrong kind of commit during a real incident. That is the
  signal to separate deployment state into its own repository, already flagged
  as the next step in ADR-0011.

## References

- [Conventional Commits](https://www.conventionalcommits.org/en/v1.0.0/)
- [Trunk Based Development](https://trunkbaseddevelopment.com/)
- [Configuring commit squashing for pull requests](https://docs.github.com/en/repositories/configuring-branches-and-merges-in-your-repository/configuring-pull-request-merges/configuring-commit-squashing-for-pull-requests)
