#!/usr/bin/env node

// This script makes sure that each provisioned Grafana alert rule follows
// the contract in docs/specs/observability-alerts-and-runbooks.md.

import { existsSync, readdirSync, readFileSync } from 'node:fs';
import { join, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';

const root = fileURLToPath(new URL('..', import.meta.url));
const alertingDir = join(root, 'deploy/overlays/local/observability/alerting');
const dashboardsDir = join(root, 'deploy/overlays/local/observability/dashboards');
const runbookBase = 'https://github.com/vasapolrittideah/flowspace-api/blob/main/docs/runbooks/';
const dashboardBase = 'http://localhost:3000/d/';

// The Alert rules table of the specification. Each rule file is the rule
// title in lowercase words separated by hyphens.
const contract = new Map([
  ['API error rate is high', 'page'],
  ['API latency is high', 'ticket'],
  ['Outbox events are stuck', 'page'],
  ['Email consumer lag is not decreasing', 'ticket'],
  ['Worker metrics are missing', 'ticket'],
  ['Email delivery fails', 'ticket'],
  ['Telemetry is dropped', 'ticket'],
  ['Host disk is almost full', 'ticket'],
]);
const labelKeys = ['severity'];
const annotationKeys = ['dashboard_url', 'runbook_url', 'summary'];

const sameKeys = (object, keys) => Object.keys(object ?? {}).sort().join(', ') === keys.join(', ');

export function checkRules(file, { runbookExists, dashboardUids }) {
  const findings = [];
  const seen = new Set();

  for (const rule of (file.groups ?? []).flatMap((group) => group.rules ?? [])) {
    const fail = (message) => findings.push(`${rule.title}: ${message}`);
    const severity = contract.get(rule.title);
    if (!severity) {
      fail('the specification has no rule with this title.');
      continue;
    }
    if (seen.has(rule.title)) fail('the title appears more than once.');
    seen.add(rule.title);

    if (!sameKeys(rule.labels, labelKeys)) {
      fail(`the labels must be ${labelKeys.join(', ')}, not ${Object.keys(rule.labels ?? {}).sort().join(', ')}.`);
    }
    if (rule.labels?.severity !== severity) fail(`severity must be ${severity}, not ${rule.labels?.severity}.`);

    const annotations = rule.annotations ?? {};
    if (!sameKeys(annotations, annotationKeys)) {
      fail(`the annotations must be ${annotationKeys.join(', ')}, not ${Object.keys(annotations).sort().join(', ')}.`);
    }
    if ('runbook_url' in annotations) {
      const runbook = `${rule.title.toLowerCase().replaceAll(' ', '-')}.md`;
      if (annotations.runbook_url !== `${runbookBase}${runbook}`) fail(`runbook_url must be ${runbookBase}${runbook}.`);
      else if (!runbookExists(runbook)) fail(`docs/runbooks/${runbook} does not exist.`);
    }
    if ('dashboard_url' in annotations) {
      const uid = annotations.dashboard_url.startsWith(dashboardBase) && annotations.dashboard_url.slice(dashboardBase.length);
      if (!dashboardUids.has(uid)) fail(`dashboard_url must be ${dashboardBase}<UID> with a UID of a Flowspace dashboard.`);
    }
  }

  return findings;
}

function main() {
  const context = {
    runbookExists: (file) => existsSync(join(root, 'docs/runbooks', file)),
    dashboardUids: new Set(readdirSync(dashboardsDir).map((file) => JSON.parse(readFileSync(join(dashboardsDir, file), 'utf8')).uid)),
  };
  const files = readdirSync(alertingDir)
    .filter((file) => file.endsWith('.json'))
    .map((file) => ({ file, json: JSON.parse(readFileSync(join(alertingDir, file), 'utf8')) }))
    .filter(({ json }) => 'groups' in json);
  const count = files.flatMap(({ json }) => json.groups.flatMap((group) => group.rules ?? [])).length;
  if (count === 0) throw new Error(`No alert rules were found in ${alertingDir}.`);

  const findings = files.flatMap(({ file, json }) => checkRules(json, context).map((finding) => `${file}: ${finding}`));
  if (findings.length > 0) {
    for (const finding of findings) console.error(finding);
    process.exitCode = 1;
    return;
  }

  console.log(`Checked ${count} alert rule(s).`);
}

if (process.argv[1] && resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  try {
    main();
  } catch (error) {
    console.error(error.message);
    process.exitCode = 1;
  }
}
