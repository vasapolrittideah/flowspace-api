---
name: flowspace-test
description: Use test-driven development for new behavior or prove a bug with a failing regression test.
---

# FlowSpace Test

Use the `test-driven-development` skill.

## New behavior

1. Read the requirements, existing test patterns, and the repository test commands.
2. Write a test that demonstrates the expected behavior and fails before implementation.
3. Implement the smallest change that passes the test.
4. Refactor while the tests pass.
5. Run the relevant regression suite and report its result.

## Bug fix

1. Write a test that reproduces the reported bug.
2. Run the test and make sure that it fails for the reported cause.
3. Implement the fix within the assigned scope.
4. Run the test and make sure that it passes.
5. Run the relevant regression suite.

Report the failure before the fix, the passing result after it, and any checks that did not run.
