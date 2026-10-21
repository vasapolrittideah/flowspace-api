#!/usr/bin/env node

// This script runs the promtool rule tests in tests/alerts/ against the queries
// of the provisioned Grafana alert rules. Each test file is named after the
// rule title in lowercase words separated by hyphens. The script writes the
// query A of that rule to rules.json in a temporary directory, and promtool
// evaluates it as a Prometheus alerting rule without a pending period. Query A
// must return a series only when the rule fires.
//
// It uses promtool from PATH, or the Prometheus image that the local cluster
// runs through Docker, as the current user so that the container can read the
// temporary directory.
//
//   node scripts/checks/test-alert-queries.mjs [rules file]

import { spawnSync } from 'node:child_process';
import { copyFileSync, mkdtempSync, readdirSync, readFileSync, rmSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { fileURLToPath } from 'node:url';

const root = fileURLToPath(new URL('../..', import.meta.url));
const testsDir = join(root, 'tests/alerts');
const rulesFile = process.argv[2] ?? join(root, 'deploy/overlays/local/observability/alerting/rules.json');
const image = 'quay.io/prometheus/prometheus:v3.15.0';

const rules = JSON.parse(readFileSync(rulesFile, 'utf8')).groups.flatMap((group) => group.rules);
const slug = (title) => title.toLowerCase().replaceAll(' ', '-');
const hasPromtool = spawnSync('promtool', ['--version']).status === 0;

let failed = false;
for (const file of readdirSync(testsDir).filter((name) => name.endsWith('.yaml'))) {
  const rule = rules.find((candidate) => `${slug(candidate.title)}.yaml` === file);
  if (!rule) throw new Error(`${file}: no provisioned rule has this title.`);
  const query = rule.data.find((part) => part.refId === 'A').model.expr;

  const dir = mkdtempSync(join(tmpdir(), 'alert-queries-'));
  try {
    writeFileSync(join(dir, 'rules.json'), JSON.stringify({ groups: [{ name: 'test', rules: [{ alert: rule.title, expr: query }] }] }));
    copyFileSync(join(testsDir, file), join(dir, 'test.yaml'));
    const command = hasPromtool
      ? ['promtool', ['test', 'rules', 'test.yaml'], { cwd: dir }]
      : ['docker', ['run', '--rm', '--user', `${process.getuid()}:${process.getgid()}`, '--entrypoint', 'promtool', '-v', `${dir}:/t`, '-w', '/t', image, 'test', 'rules', 'test.yaml'], {}];
    const result = spawnSync(command[0], command[1], { ...command[2], encoding: 'utf8' });
    console.log(`${file}: ${result.status === 0 ? 'passed' : 'failed'}`);
    if (result.status !== 0) {
      console.error(result.stdout, result.stderr, result.error?.message ?? '');
      failed = true;
    }
  } finally {
    rmSync(dir, { recursive: true, force: true });
  }
}
if (failed) process.exitCode = 1;
