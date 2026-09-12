import assert from 'node:assert/strict';
import test from 'node:test';

import { coverageForAddedLines, floorFindings, parseDiff } from './constraint-check.mjs';

test('floor guard catches attempts to weaken the bar', () => {
  const suppression = ['//', 'nolint:gosec'].join('');
  const skip = ['t', '.Skip("later")'].join('');
  const diff = [
    'diff --git a/service_test.go b/service_test.go',
    '--- a/service_test.go',
    '+++ b/service_test.go',
    '@@ -1,1 +1,3 @@',
    '-t.Fatal("expected failure")',
    `+${suppression}`,
    `+${skip}`,
    '+return nil',
    'diff --git a/CONSTRAINTS.md b/CONSTRAINTS.md',
    '--- a/CONSTRAINTS.md',
    '+++ b/CONSTRAINTS.md',
    '@@ -10,1 +10,2 @@',
    '-| C1 | Coverage | Changed executable lines ≥ 80% | check | task end |',
    '+| C1 | Coverage | Changed executable lines ≥ 70% | check | task end |',
    '+| E1 | Coverage | legacy | later | owner | 2026-12-01 |',
  ].join('\n');

  const rules = floorFindings(parseDiff(diff)).map(({ rule }) => rule);

  assert.deepEqual(rules.sort(), ['assertion-removed', 'new-exception', 'silenced-checker', 'test-made-easier', 'threshold-lowered'].sort());
});

test('changed-line coverage counts instrumented added Go lines', () => {
  const diff = [
    'diff --git a/service.go b/service.go',
    '--- a/service.go',
    '+++ b/service.go',
    '@@ -1,0 +2,5 @@',
    '+covered()',
    '+coveredAgain()',
    '+coveredThird()',
    '+coveredFourth()',
    '+missed()',
  ].join('\n');
  const profile = [
    'mode: set',
    'example/service.go:2.1,5.20 4 1',
    'example/service.go:6.1,6.20 1 0',
  ].join('\n');

  assert.deepEqual(coverageForAddedLines(parseDiff(diff), profile), { covered: 4, instrumented: 5, percent: 80 });
});
