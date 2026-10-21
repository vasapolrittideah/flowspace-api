import assert from 'node:assert/strict';
import { spawnSync } from 'node:child_process';
import test from 'node:test';
import { fileURLToPath } from 'node:url';

import { checkRules } from './check-alert-rules.mjs';

const script = fileURLToPath(new URL('check-alert-rules.mjs', import.meta.url));
const runbookBase = 'https://github.com/vasapolrittideah/flowspace-api/blob/main/docs/runbooks/';
const context = {
  runbookExists: (file) => file === 'api-error-rate-is-high.md',
  dashboardUids: new Set(['flowspace-service-health']),
};

const rule = (changes = {}) => ({
  title: 'API error rate is high',
  labels: { severity: 'page' },
  annotations: {
    summary: 'API error rate is high: {{ humanizePercentage $values.A.Value }} of requests fail.',
    runbook_url: `${runbookBase}api-error-rate-is-high.md`,
    dashboard_url: 'http://localhost:3000/d/flowspace-service-health',
  },
  ...changes,
});

const file = (...rules) => ({ apiVersion: 1, groups: [{ name: 'flowspace', rules }] });

test('accepts a rule that follows the contract', () => {
  assert.deepEqual(checkRules(file(rule()), context), []);
});

test('rejects a rule without a runbook', () => {
  const { runbook_url: _, ...annotations } = rule().annotations;
  assert.deepEqual(checkRules(file(rule({ annotations })), context), [
    'API error rate is high: the annotations must be dashboard_url, runbook_url, summary, not dashboard_url, summary.',
  ]);
});

test('rejects a runbook_url that names a missing file', () => {
  const annotations = { ...rule().annotations, runbook_url: `${runbookBase}api-latency-is-high.md` };
  assert.deepEqual(checkRules(file(rule({ annotations })), context), [
    `API error rate is high: runbook_url must be ${runbookBase}api-error-rate-is-high.md.`,
  ]);
});

test('rejects a runbook_url whose file does not exist', () => {
  const findings = checkRules(file(rule()), { ...context, runbookExists: () => false });
  assert.deepEqual(findings, ['API error rate is high: docs/runbooks/api-error-rate-is-high.md does not exist.']);
});

test('rejects an unknown severity', () => {
  assert.deepEqual(checkRules(file(rule({ labels: { severity: 'critical' } })), context), [
    'API error rate is high: severity must be page, not critical.',
  ]);
});

test('rejects a severity that differs from the contract', () => {
  assert.deepEqual(checkRules(file(rule({ labels: { severity: 'ticket' } })), context), [
    'API error rate is high: severity must be page, not ticket.',
  ]);
});

test('rejects an extra label', () => {
  assert.deepEqual(checkRules(file(rule({ labels: { severity: 'page', team: 'identity' } })), context), [
    'API error rate is high: the labels must be severity, not severity, team.',
  ]);
});

test('rejects a missing summary and an extra annotation', () => {
  const { summary: _, ...annotations } = rule().annotations;
  assert.deepEqual(checkRules(file(rule({ annotations: { ...annotations, description: 'x' } })), context), [
    'API error rate is high: the annotations must be dashboard_url, runbook_url, summary, not dashboard_url, description, runbook_url.',
  ]);
});

test('rejects a dashboard_url without a known dashboard UID', () => {
  const annotations = { ...rule().annotations, dashboard_url: 'http://localhost:3000/d/flowspace-unknown' };
  assert.deepEqual(checkRules(file(rule({ annotations })), context), [
    'API error rate is high: dashboard_url must be http://localhost:3000/d/<UID> with a UID of a Flowspace dashboard.',
  ]);
});

test('rejects a rule title outside the contract', () => {
  assert.deepEqual(checkRules(file(rule({ title: 'Pod restarts are high' })), context), [
    'Pod restarts are high: the specification has no rule with this title.',
  ]);
});

test('rejects a rule title that appears twice', () => {
  assert.deepEqual(checkRules(file(rule(), rule()), context), ['API error rate is high: the title appears more than once.']);
});

test('accepts the provisioned rules of the repository', () => {
  const result = spawnSync(process.execPath, [script], { encoding: 'utf8' });
  assert.equal(result.status, 0, result.stderr);
  assert.match(result.stdout, /Checked \d+ alert rule\(s\)/);
});
