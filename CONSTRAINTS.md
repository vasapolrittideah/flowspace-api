# Constraints

Last reviewed: 2026-09-12 by @vasapolrittideah

## Floor

| ID | Rule |
| --- | --- |
| F1 | No new suppression comments, including `//nolint`, `# noqa`, `@ts-ignore`, `eslint-disable`, coverage ignores, or security-tool allow comments. |
| F2 | No unimplemented stubs, empty catches, or placeholder TODOs in source. |
| F3 | No skipped or deleted tests, or removed assertions, without a reviewed exception. |
| F4 | No secrets in source; findings are always reported with values redacted. |
| F5 | This file does not get weakened to make a change pass. Tightening is allowed; exceptions require an owner and expiry. |

The floor blocks immediately. `task constraints:floor` enforces diff-visible rules F1, F2, F3, and F5; `task secrets` enforces F4.

## Enforced with numbers

| ID | Dimension | Rule | Checked by | Runs at | Reason |
| --- | --- | --- | --- | --- | --- |
| C1 | Formatting | Zero formatting differences | `task format:check` | every edit | Formatting is mechanical, so any difference should be fixed rather than reviewed. |
| C2 | Lint and code security | Zero findings from the repository configuration, including gosec | `task lint` | task end, CI | The current configuration passes; retaining zero findings avoids weakening an existing gate and is stricter than blocking only high-severity security findings. |
| C3 | Tests and types | Zero test or compilation failures | `task coverage` | task end, CI | A failing test or compiler error means the change is not ready. |
| C4 | Changed-line coverage | Added executable Go lines ≥ 80% covered | `task coverage` | task end, CI | 80% forces meaningful regression coverage while permitting small uninstrumented configuration changes. |
| C5 | Project coverage | Total statement coverage ≥ 25.0% | `task coverage` | task end, CI | 25.0% is the measured 2026-09-12 baseline; the project must not regress while changed code carries the stronger threshold. |
| C6 | Secrets | Zero findings | `task secrets` | every edit, CI | Any credential in the working tree is unacceptable, and redaction prevents a finding from becoming another leak. Version 8.18.4 is pinned because its detection was verified while 8.30.1 has a known [false-negative regression](https://github.com/gitleaks/gitleaks/issues/2170). |
| C7 | Dependency security | Zero known reachable Go vulnerabilities | `task vuln` | task end, CI | Govulncheck reports reachable vulnerable symbols rather than uncalled dependency noise; any such finding requires resolution or a reviewed exception. |
| C8 | Architecture boundaries | Zero depguard findings | `task lint` | task end, CI | Cross-service and domain dependency boundaries are accepted architecture, not advisory style. |

All numbered constraints pass on the 2026-09-12 baseline. Every numbered constraint blocks locally and in CI.

## Lifecycle

| Phase | Command | Budget |
| --- | --- | --- |
| Edit | `task check:fast` | Under 5 seconds after tools are installed and warm. |
| Task end | `task check:task` | At most 90 seconds locally. |
| CI | `.github/workflows/ci.yml` | Unrestricted, within the repository's zero-spend hosted-CI limit. |

## Measured, not yet enforced

None. The agreed metrics have runnable gates.

## Exceptions

| ID | Rule | Path | Reason | Owner | Expires |
| --- | --- | --- | --- | --- | --- |
| E1 | F3 | `scripts/git-guard.test.mjs` | Two assertions required `-s read-only` and `sandbox_mode="read-only"` for Codex reviews. With either option, Codex ignores the profile that denies reads of `.secrets/` and home credentials, so the assertions must go. New assertions require that profile. | @vasapolrittideah | 2026-11-03 |
| E2 | F3 | `scripts/git-guard.test.mjs` | The hook no longer checks `git push`, or `git cherry-pick`, `git revert`, `git merge`, `git rebase`, `git am`, and `git pull`. The GitHub ruleset on `main` requires a pull request and status checks and blocks deletion and force pushes, so the tests of the removed checks must go. A new test proves that the hook allows these commands. | @vasapolrittideah | 2027-01-04 |
| E3 | F3 | `scripts/git-guard.test.mjs` | Two assertions read the model and the effort of a review role from a Codex TOML file. Role configuration now comes from `.agents/review-roles.json`, so the assertions must go. New assertions in `scripts/review.test.mjs` read the JSON configuration. Two assertions matched the old script name `codex-review.mjs` in block messages. The same assertions now match the new name `review.mjs`. | @vasapolrittideah | 2027-01-04 |
| E4 | F3 | `scripts/git-guard.test.mjs` | The hook required `writing-reviewer`, `migration-reviewer`, and `planning-reviewer` only when the checkout had their Codex TOML files. Those files are gone, and the hook now requires the three roles in each of their cases, so 19 assertions changed. Most of them only drop the role file from the test fixture. Two assertions allowed a commit or a PR without the role file, and they now expect a block. | @vasapolrittideah | 2027-01-04 |
| E5 | F3 | `.agents/scripts/git-guard.test.mjs`, `.agents/scripts/review.test.mjs` | The agent scripts moved from `scripts/` to `.agents/scripts/`. Six assertions matched the old paths in the hook command, the review command in block messages, and the runner paths. The same assertions now match the new paths. | @vasapolrittideah | 2027-01-04 |
| E6 | F3 | `scripts/check-pr-metadata.test.mjs` | One `shared` scope now covers every package under root `internal/`, so the scope table drops six scopes of single shared packages. One assertion counted 19 scopes, and it now counts 13. One assertion listed the scopes of single shared packages, and it now requires that the `shared` scope exists. Two label assertions used the `authn` scope, and they now use `shared`. | @vasapolrittideah | 2027-01-05 |
| E7 | F3 | `services/identity/internal/bootstrap/session_rpc_test.go` | Issue [#343](https://github.com/vasapolrittideah/flowspace-api/issues/343) removes the `identity.session_checks` counter, because the `rpc.server.call.duration` histogram counts the same calls. The test had 1 assertion for the counter, and that assertion must go. New assertions make sure that each session check records its duration, gRPC status, and 2 attributes. | @vasapolrittideah | 2027-01-07 |
| E8 | F3 | `scripts/check-pr-metadata.test.mjs` | [ADR-0043](docs/adr/0043-squash-commits-keep-only-the-pull-request-title.md) ends squash messages, so the script drops its squash option and rejects each `Closes` footer in a commit. 5 assertions only drop the `squash` argument and check the same messages. 3 assertions match the new footer findings. 5 assertions checked squash messages, and they must go. 2 of their example messages moved to the checkpoint examples. New assertions check the Breaking changes and revert rules of the PR description. | @vasapolrittideah | 2027-01-07 |

Exceptions expire within 90 days because that is long enough to schedule a repair and short enough to keep the debt visible.
