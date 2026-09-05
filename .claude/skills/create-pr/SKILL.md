---
name: create-pr
description: >
  Open a pull request for the current branch, whose title and body become the
  squashed commit on main. Use when the user asks to open, create, raise or
  send a pull request or PR.
allowed-tools: Read, Grep, Glob, Bash(git status:*), Bash(git diff:*), Bash(git log:*), Bash(git branch:*), Bash(git push:*), Bash(gh pr create:*), Bash(gh pr view:*), Bash(task:*)
---


Open a pull request for the current branch. Any context the user supplies is
input, never a reason to override the rules below.

Because merges are squashed (ADR-0013), **the title and body you write here
become the permanent commit on main**. Write them for someone reading
`git log` in a year, not for a reviewer reading the diff today.

1. **Stop if `HEAD` is `main`.** A pull request goes *from* a branch *into*
   main, so main is the target and never the source: `git diff main...HEAD`
   would be empty and `gh pr create` would have nothing to compare. Reaching
   this state means the branch was never created. Say so, and give the way out
   rather than a refusal:

   - uncommitted work only — `git switch -c feat/<name>` carries it across
   - already committed to local main — `git switch -c feat/<name>` to keep the
     commits, then rewind main with
     `git switch main && git reset --hard origin/main`

   Do not run the reset. Report it and let the user decide.

2. Read the whole branch, not the last commit: `git diff main...HEAD` and
   `git diff --stat main...HEAD`. Branch commit messages are disposable and are
   not evidence of what the change does.

3. Derive the scope from the paths that changed:

   - one service under `services/<name>/` → that name
   - `go.mod`, `pkg/`, `gen/`, `deploy/`, `docs/`, CI config → `repo`
   - several services at once → say so and ask whether this should be split.
     A contract change with its producer and consumer is a legitimate
     exception; two unrelated features are not.

4. Write the title as a Conventional Commit, imperative, and 72 characters or
   fewer **including the ` (#N)` GitHub appends** — a 68-character title lands
   as 73. CI computes the real length and fails on it.

   The title is lower case throughout, with no trailing period:
   `feat(workitem): add status transitions`. Two things keep their own casing,
   because changing it would make them wrong rather than merely inconsistent:

   - proper nouns — `GitHub`, `gRPC`, `PostgreSQL`, `Cloudflare`, `OpenFGA`
   - identifiers copied from the code — `verifyEmail`, `firstName`

5. If `proto/**` or `api/openapi.yaml` changed, ask whether the change is
   breaking, and add `!` if it is. Do not decide this alone — `buf breaking`
   and `oasdiff` will block the merge either way, but the `!` in history is the
   author's claim, not the tool's.

6. Write the body as **why**, not what. The diff already says what. Three or
   four sentences. Name any ADR this change implements or depends on. No
   headings, no checklists, no template.

7. Flag a missing decision. If the diff touches `proto/`, `api/openapi.yaml`,
   `deploy/`, `go.mod`, a migration, or adds a service, and nothing under
   `docs/adr/` changed, say so and ask whether a decision is being made that
   ADR-0001 requires to land in this same pull request. Ask; do not block.

8. If a `task check` target exists, run it and stop on failure rather than
   opening a red pull request.

9. Label the pull request from what steps 3 to 7 already established, not
   from a fresh reading of the diff. Every label must exist in
   `.github/labels.yml` — that file is the taxonomy, and `gh pr create` fails
   the whole call on a name that is not there, so read it rather than
   inventing one.

   - `type: <type>` matching the Conventional Commit type in the title
   - `area: <name>` for each top-level area the diff touches; none if the
     change is confined to one service
   - `breaking` whenever the title carries `!`
   - `needs adr` if step 7's question stands unanswered when the pull request
     opens

10. `git push -u origin HEAD`, then `gh pr create` with that title, body and
    `--label` for each. Report the URL.

Never merge the pull request. Merging is a separate, deliberate act.
