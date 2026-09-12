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

All numbered constraints pass on the 2026-09-12 baseline. C3, C4, C5, and C7 warn locally and in CI through 2026-09-25 and block beginning 2026-09-26; the floor, secret scan, and existing formatting, lint, code-security, and architecture gates block immediately.

## Lifecycle

| Phase | Command | Budget |
| --- | --- | --- |
| Edit | `task check:fast` | Under 5 seconds after tools are installed and warm. |
| Task end | `task check:task` | At most 90 seconds locally. |
| CI | `.github/workflows/constraints.yml` plus the existing lint workflow | Unrestricted, within the repository's zero-spend hosted-CI limit. |

## Measured, not yet enforced

None. The agreed metrics have runnable gates.

## Exceptions

| ID | Rule | Path | Reason | Owner | Expires |
| --- | --- | --- | --- | --- | --- |

Exceptions expire within 90 days because that is long enough to schedule a repair and short enough to keep the debt visible.
