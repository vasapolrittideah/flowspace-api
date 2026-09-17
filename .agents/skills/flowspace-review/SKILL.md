---
name: flowspace-review
description: Review current changes for correctness, readability, architecture, security, and performance.
---

# FlowSpace Review

Use the `code-review-and-quality` skill.

Review the assigned change. If no target is given, review the staged changes or recent commits. Read the task or spec before the diff.

Assess these dimensions:

1. Correctness: Compare behavior with the requirements, edge cases, and test coverage.
2. Readability: Assess names, control flow, and the explanation needed to understand the code.
3. Architecture: Compare boundaries and dependencies with the accepted repository design.
4. Security: Assess inputs, secrets, authentication, and authorization. Use `security-and-hardening` when a deeper pass is needed.
5. Performance: Look for unnecessary work, repeated queries, and unbounded operations. Use `performance-optimization` when needed.

Group findings as Critical, Important, or Suggestion. Give each finding a file path, line number, and a specific fix. Report unresolved verification gaps. Return the review without changing files.
