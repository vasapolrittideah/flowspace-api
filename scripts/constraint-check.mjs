#!/usr/bin/env node

import { readFileSync } from 'node:fs';
import { spawnSync } from 'node:child_process';
import { pathToFileURL } from 'node:url';

const sourceFile = /\.(?:go|js|mjs|cjs|ts|tsx|py|sh|bash|zsh|ya?ml)$/;
const suppressionPattern = new RegExp([
  ['@ts', 'ignore'].join('-'),
  ['@ts', 'nocheck'].join('-'),
  ['eslint', 'disable'].join('-'),
  ['biome', 'ignore'].join('-'),
  '# *noqa',
  '# *type: *ignore',
  ['istanbul', 'ignore'].join(' '),
  'nosem' + 'grep',
  ['gitleaks', 'allow'].join(':'),
  ['Stryker', 'disable'].join(' '),
  '// *no' + 'lint',
].join('|'));
const stubPattern = new RegExp(['throw new (Error|NotImplemented).*[Nn]ot implemented', 'panic\\([^)]*[Nn]ot implemented', 'catch\\s*\\([^)]*\\)\\s*\\{\\s*\\}', 'catch\\s*\\{\\s*\\}', '\\bTO' + 'DO\\b', '\\bFIX' + 'ME\\b'].join('|'));
const skipPattern = new RegExp(['\\.(skip|todo)\\b', '\\bxit\\(', '\\bxdescribe\\(', '@pytest\\.mark\\.skip', 't\\.Skip(?:f)?\\('].join('|'));
const assertionPattern = /\b(?:assert|expect|should)\b|\bt\.(?:Error|Errorf|Fail|FailNow|Fatal|Fatalf)\(/;

export function parseDiff(diff) {
  const added = [];
  const removed = [];
  const deletedTests = [];
  let oldFile = '';
  let file = '';
  let newLine = 0;

  for (const line of diff.split('\n')) {
    if (line.startsWith('--- ')) {
      oldFile = normalizeDiffPath(line.slice(4));
    } else if (line.startsWith('+++ ')) {
      const nextFile = normalizeDiffPath(line.slice(4));
      if (nextFile === '/dev/null') {
        file = oldFile;
        if (isTestFile(file)) deletedTests.push(file);
      } else {
        file = nextFile;
      }
    } else if (line.startsWith('@@ ')) {
      newLine = Number(line.match(/\+(\d+)/)?.[1] ?? 0);
    } else if (line.startsWith('+') && !line.startsWith('+++')) {
      added.push({ file, line: newLine, text: line.slice(1) });
      newLine += 1;
    } else if (line.startsWith('-') && !line.startsWith('---')) {
      removed.push({ file, text: line.slice(1) });
    } else if (line.startsWith(' ')) {
      newLine += 1;
    }
  }

  return { added, removed, deletedTests };
}

export function floorFindings({ added, removed, deletedTests }) {
  const findings = [];
  const flag = (rule, file) => findings.push({ rule, file });

  for (const { file, text } of added) {
    if (sourceFile.test(file) && suppressionPattern.test(text)) flag('silenced-checker', file);
    if (sourceFile.test(file) && stubPattern.test(text)) flag('unfinished-work', file);
    if (isTestFile(file) && skipPattern.test(text)) flag('test-made-easier', file);
    if (file === 'CONSTRAINTS.md' && /^\|\s*E\d+\s*\|/.test(text)) flag('new-exception', file);
  }

  for (const { file, text } of removed) {
    if (isTestFile(file) && !deletedTests.includes(file) && assertionPattern.test(text)) flag('assertion-removed', file);
  }
  for (const file of deletedTests) flag('test-deleted', file);

  const removedRows = constraintRows(removed);
  const addedRows = constraintRows(added);
  for (const [id, oldRow] of removedRows) {
    const newRow = addedRows.get(id);
    if (!newRow) {
      if (id.startsWith('F')) flag('floor-rule-removed', 'CONSTRAINTS.md');
      if (id.startsWith('C')) flag('constraint-removed', 'CONSTRAINTS.md');
      continue;
    }
    if (weakensThreshold(oldRow, newRow)) flag('threshold-lowered', 'CONSTRAINTS.md');
  }

  return findings;
}

export function coverageForAddedLines({ added }, profile) {
  const blocks = parseCoverage(profile);
  const lines = new Map();

  for (const { file, line } of added) {
    if (!file.endsWith('.go') || file.endsWith('_test.go') || file.startsWith('gen/')) continue;
    const matching = blocks.filter((block) => coveragePathMatches(block.file, file) && line >= block.start && line <= block.end);
    if (matching.length === 0) continue;
    lines.set(`${file}:${line}`, matching.some((block) => block.count > 0));
  }

  const instrumented = lines.size;
  const covered = [...lines.values()].filter(Boolean).length;
  return { covered, instrumented, percent: instrumented === 0 ? null : (covered / instrumented) * 100 };
}

export function totalCoverage(profile) {
  const unique = new Map();
  for (const block of parseCoverage(profile)) {
    const key = `${block.file}:${block.startColumn},${block.start}:${block.endColumn},${block.end}`;
    const current = unique.get(key);
    if (!current || block.count > current.count) unique.set(key, block);
  }
  const blocks = [...unique.values()];
  const statements = blocks.reduce((sum, block) => sum + block.statements, 0);
  const covered = blocks.reduce((sum, block) => sum + (block.count > 0 ? block.statements : 0), 0);
  return statements === 0 ? 0 : (covered / statements) * 100;
}

function normalizeDiffPath(value) {
  if (value === '/dev/null') return value;
  return value.replace(/^[ab]\//, '').split('\t', 1)[0];
}

function isTestFile(file) {
  return /(?:_test\.go|(?:^|\/)(?:test|tests)\/|\.(?:test|spec)\.)/.test(file);
}

function constraintRows(lines) {
  const rows = new Map();
  for (const { file, text } of lines) {
    if (file !== 'CONSTRAINTS.md') continue;
    const id = text.match(/^\|\s*([FCE]\d+)\s*\|/)?.[1];
    if (id) rows.set(id, text);
  }
  return rows;
}

function weakensThreshold(oldRow, newRow) {
  const oldThreshold = oldRow.match(/(≥|≤|>=|<=)\s*(\d+(?:\.\d+)?)/);
  const newThreshold = newRow.match(/(≥|≤|>=|<=)\s*(\d+(?:\.\d+)?)/);
  if (oldThreshold && newThreshold && oldThreshold[1] === newThreshold[1]) {
    const oldValue = Number(oldThreshold[2]);
    const newValue = Number(newThreshold[2]);
    if ((oldThreshold[1].includes('>') || oldThreshold[1] === '≥') && newValue < oldValue) return true;
    if ((oldThreshold[1].includes('<') || oldThreshold[1] === '≤') && newValue > oldValue) return true;
  }

  const severities = ['low', 'medium', 'high', 'critical'];
  const oldSeverity = severities.findIndex((severity) => oldRow.toLowerCase().includes(severity));
  const newSeverity = severities.findIndex((severity) => newRow.toLowerCase().includes(severity));
  return oldSeverity >= 0 && newSeverity > oldSeverity;
}

function parseCoverage(profile) {
  const blocks = [];
  for (const line of profile.split('\n').slice(1)) {
    const match = line.match(/^(.+):(\d+)\.(\d+),(\d+)\.(\d+)\s+(\d+)\s+(\d+)$/);
    if (!match) continue;
    blocks.push({
      file: match[1],
      start: Number(match[2]),
      startColumn: Number(match[3]),
      end: Number(match[4]),
      endColumn: Number(match[5]),
      statements: Number(match[6]),
      count: Number(match[7]),
    });
  }
  return blocks;
}

function coveragePathMatches(profilePath, file) {
  return profilePath === file || profilePath.endsWith(`/${file}`);
}

function git(args, accepted = [0]) {
  const result = spawnSync('git', args, { encoding: 'utf8' });
  if (!accepted.includes(result.status)) return null;
  return result.stdout;
}

function collectDiff(base) {
  const mergeBase = git(['merge-base', base, 'HEAD'])?.trim();
  if (!mergeBase) throw new Error(`no merge base against ${base}`);

  const tracked = git(['diff', '--unified=0', mergeBase, '--']);
  if (tracked === null) throw new Error('could not read tracked changes');
  const untrackedFiles = (git(['ls-files', '--others', '--exclude-standard']) ?? '').split('\n').filter(Boolean);
  const untracked = untrackedFiles.map((file) => git(['diff', '--no-index', '--unified=0', '/dev/null', file], [0, 1]) ?? '').join('\n');
  return parseDiff(`${tracked}\n${untracked}`);
}

function option(name, fallback) {
  const index = process.argv.indexOf(name);
  return index >= 0 ? process.argv[index + 1] : fallback;
}

function runFloor() {
  let diff;
  try {
    diff = collectDiff(option('--base', 'origin/main'));
  } catch (error) {
    console.error(`constraint-check: ${error.message}`);
    process.exit(2);
  }

  const findings = floorFindings(diff);
  if (findings.length === 0) {
    console.log('constraint floor: clean');
    return;
  }
  console.error(`constraint floor: ${findings.length} violation(s)`);
  for (const { rule, file } of findings) console.error(`  [${rule}] ${file}`);
  process.exit(1);
}

function runCoverage() {
  const profilePath = option('--profile');
  if (!profilePath) {
    console.error('constraint-check: --profile is required');
    process.exit(2);
  }

  let diff;
  let profile;
  try {
    diff = collectDiff(option('--base', 'origin/main'));
    profile = readFileSync(profilePath, 'utf8');
  } catch (error) {
    console.error(`constraint-check: ${error.message}`);
    process.exit(2);
  }

  const total = totalCoverage(profile);
  const changed = coverageForAddedLines(diff, profile);
  const totalMinimum = Number(option('--total-min', '25'));
  const changedMinimum = Number(option('--changed-min', '80'));
  console.log(`project coverage: ${total.toFixed(1)}% (minimum ${totalMinimum.toFixed(1)}%)`);
  console.log(changed.instrumented === 0 ? 'changed-line coverage: not applicable' : `changed-line coverage: ${changed.percent.toFixed(1)}% (${changed.covered}/${changed.instrumented}, minimum ${changedMinimum.toFixed(1)}%)`);
  if (total + Number.EPSILON < totalMinimum || (changed.instrumented > 0 && changed.percent + Number.EPSILON < changedMinimum)) process.exit(1);
}

if (process.argv[1] && import.meta.url === pathToFileURL(process.argv[1]).href) {
  const command = process.argv[2];
  if (command === 'floor') runFloor();
  else if (command === 'coverage') runCoverage();
  else {
    console.error('usage: constraint-check.mjs <floor|coverage> [options]');
    process.exit(2);
  }
}
