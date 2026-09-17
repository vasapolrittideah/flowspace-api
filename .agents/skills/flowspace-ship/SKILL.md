---
name: flowspace-ship
description: Assess launch readiness with specialist reviews, a go or no-go decision, and a rollback plan.
---

# FlowSpace Ship

Use the `shipping-and-launch` skill. Assess the assigned change or the current PR. This workflow produces a readiness report. It does not authorize a merge, deployment, or publication.

## Specialist reviews

Run these reviews in parallel with the available Codex subagent tools:

1. `code-reviewer`: Review correctness, readability, architecture, security, and performance.
2. `security-auditor`: Review vulnerabilities, trust boundaries, secrets, authentication, authorization, and dependency risks.
3. `test-engineer`: Analyze coverage gaps for expected behavior, edge cases, error paths, and concurrency.

Use the matching custom agent definitions in `.codex/agents/` when the tool supports named roles. Otherwise, give each subagent the matching `.agents/agents/<name>.md` instructions. Keep each reviewer read-only and prevent further delegation. Give all reviewers the same change scope and wait for their reports.

If subagent tools are unavailable, perform the reviews sequentially and disclose that limitation. Do not present sequential work as parallel work.

Skip separate subagents only when all these conditions hold: at most two changed files, fewer than 50 changed lines, and no changes to authentication, payments, data access, or environment configuration. Apply the same review dimensions directly.

## Combined assessment

1. Combine code findings and failing tests, lint, or builds. Remove duplicate findings.
2. Treat Critical or High security findings as launch blockers.
3. Assess performance evidence. For browser changes, include relevant browser performance results.
4. For UI changes, assess keyboard access, screen reader support, and contrast.
5. Assess environment configuration, migrations, monitoring, and feature flags within the change scope.
6. Assess documentation, accepted decisions, and compatibility effects.

## Decision

Return GO or NO-GO with blockers, recommended fixes, accepted risks, a rollback plan, and the full specialist reports. Identify the source reviewer for each finding.

Include rollback triggers, exact recovery steps, and a recovery time objective. If recovery time is unmeasured, state that fact.

A GO decision requires a rollback plan. A Critical finding makes the default decision NO-GO unless the user explicitly accepts that risk. Report unavailable checks and reviews as limitations.
