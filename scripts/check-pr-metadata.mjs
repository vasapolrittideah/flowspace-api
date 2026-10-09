#!/usr/bin/env node
// Checks the rules of the branch name, commit message, pull request, and label conventions
// that need no judgment, so that reviewers can skip them. The conventions stay the source of
// each rule: this script reads the types and scopes from the commit message convention and
// the labels from .github/labels.json. It checks these rules:
//
// - Subject lines of the PR title and the checkpoint commits: the format, a known type and
//   scope, at most 72 characters, a lowercase first letter, and no final period.
// - The branch name format, and a branch type that matches the PR title.
// - Commit messages: the blank line after the subject, body lines of at most 72 characters,
//   the format, place, and order of Issue footers, no Closes footer, and the spelling, place,
//   and order of Co-authored-by trailers.
// - Labels: known labels, the type label, the area label of a scope, breaking, and migration.
// - The PR description: the template headings, no HTML comments, Breaking changes that are not
//   n/a when the title has !, What changed that starts with "Reverts <full SHA>." when the
//   title has the revert type, the format and order of Related issues, and the format and
//   order of Follow-up tasks.
// - The prose of the PR description and of the checkpoint commit bodies. The prose
//   has none of the modals, contractions, semicolons, em dashes, present perfect forms, or
//   filler words that the simple-english skill forbids. Code, URLs, HTML comments, and headings
//   are not prose. The convention treats the sentence limits of the skill as targets, so the
//   script does not count words.
//
// PRs that renovate[bot] opens skip the branch, commit, and description checks, because
// Renovate writes them.
//
//   node scripts/check-pr-metadata.mjs [--base <ref>] [--head <ref>] [--branch <name>]
//     [--author <login>] [--title <text>] [--labels <a,b>] [--body-file <file>]

import { spawnSync } from 'node:child_process';
import { randomUUID } from 'node:crypto';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { parseArgs } from 'node:util';

const SUBJECT = /^([a-z]+)(?:\(([^()]*)\))?(!)?: (.*)$/;
const ISSUE_FOOTER = /^(Closes|Refs): #(\d+)$/;
// A line that refers to an Issue, such as `Refs: #12` or `Fixes #12`.
const ISSUE_KEYWORD = /^(closes|refs|fixes|resolves):?\s*#?\d+/i;
const TRAILER_KEY = /^co-authored-by:/i;
const MIGRATION_PATH = /^services\/[^/]+\/db\/migrations\//;
const PR_HEADINGS = ['What changed', 'Why', 'Breaking changes', 'Related issues', 'Review notes', 'Risks or limitations', 'Follow-up tasks'];
// GitHub links a full SHA in the description to the reverted commit.
const REVERT_LINE = /^Reverts [0-9a-f]{40}\./;
const MAX_LINE = 72;
// Rules of the simple-english skill that a pattern can find. Each match is a finding.
const PROSE_RULES = [
  [/\b(should|would|may|might|could)\b/gi, (word) => `"${word}" is a modal that simple-english forbids. Use can, will, or must`],
  [/\b[a-z]+(n['’]t|['’](re|ve|ll|m|d))\b|\b(it|that|there|here|what|let|he|she|who|where|when|why|how)['’]s\b/gi, (word) => `"${word}" is a contraction`],
  [/;/g, () => 'a semicolon joins two sentences. Write two sentences'],
  [/—/g, () => 'an em dash joins two parts. Write two sentences or name the relation'],
  [/\b(has|have) been\b/gi, (words) => `"${words}" is the present perfect. Use a simple tense`],
  [/\b(simply|seamlessly|robust|powerful|comprehensive|leverage|crucial|in order to|it is worth noting|in conclusion)\b/gi, (words) => `"${words}" carries no fact`],
];

// Returns the first-column codes of the table under `heading`, with each row's other cells.
function tableRows(markdown, heading) {
  const lines = markdown.split('\n');
  const start = lines.indexOf(heading);
  const rows = [];
  for (let i = start + 1; start >= 0 && i < lines.length && !/^#{1,3} /.test(lines[i]); i += 1) {
    const cells = lines[i].split('|').slice(1, -1).map((cell) => cell.trim());
    const code = cells[0]?.match(/^`([^`]+)`$/)?.[1];
    if (code) {
      rows.push({ code, cells });
    }
  }
  return rows;
}

// Reads the types, scopes, and labels that the conventions allow.
export function loadRules(root) {
  const commits = readFileSync(`${root}/docs/conventions/commit-messages.md`, 'utf8');
  const types = tableRows(commits, '### Types');
  const scopes = tableRows(commits, '### Scopes');
  if (types.length === 0 || scopes.length === 0) {
    throw new Error('Cannot read the Types and Scopes tables of docs/conventions/commit-messages.md');
  }
  return {
    types: new Set(types.map((row) => row.code)),
    scopes: new Set(scopes.map((row) => row.code)),
    labels: new Set(JSON.parse(readFileSync(`${root}/.github/labels.json`, 'utf8')).map((label) => label.name)),
  };
}

// Returns the parts of a subject line and its findings.
export function checkSubject(subject, rules) {
  const match = subject.match(SUBJECT);
  if (!match) {
    return { findings: [`"${subject}" does not have the form <type>(<scope>): <description>`] };
  }
  const [, type, scope, bang, description] = match;
  const findings = [];
  if (!rules.types.has(type)) {
    findings.push(`type "${type}" is not in the commit message convention`);
  }
  if (scope !== undefined && !rules.scopes.has(scope)) {
    findings.push(`scope "${scope}" is not in the scope table`);
  }
  if (subject.length > MAX_LINE) {
    findings.push(`the subject line has ${subject.length} characters, more than ${MAX_LINE}`);
  }
  if (!/^[a-z]/.test(description)) {
    findings.push('the description does not start with a lowercase letter');
  }
  if (description.endsWith('.')) {
    findings.push('the description ends with a period');
  }
  return { type, scope, breaking: bang === '!', findings };
}

export function checkBranch(branch, titleType) {
  const match = branch.match(/^([a-z]+)\/([a-z0-9]+(?:-[a-z0-9]+)*)$/);
  if (!match) {
    return [`branch "${branch}" does not have the form <type>/<lowercase-words-with-hyphens>`];
  }
  if (titleType && match[1] !== titleType) {
    return [`branch type "${match[1]}" differs from the PR title type "${titleType}"`];
  }
  return [];
}

// Checks a commit message: the subject, the body lines, the Issue footers, and the trailers.
export function checkMessage(message, rules) {
  const lines = message.replace(/\r\n/g, '\n').replace(/\n+$/, '').split('\n');
  const findings = checkSubject(lines[0], rules).findings;
  if (lines.length > 1 && lines[1] !== '') {
    findings.push('no blank line after the subject');
  }
  const footers = [];
  const trailers = [];
  let fence = '';
  const kinds = lines.map((line) => {
    const marker = line.match(/^\s*(`{3,}|~{3,})/)?.[1]?.[0] ?? '';
    if (marker && (!fence || fence === marker)) {
      fence = fence ? '' : marker;
      return 'code';
    }
    if (fence) return 'code';
    if (TRAILER_KEY.test(line)) return 'trailer';
    if (ISSUE_KEYWORD.test(line)) return 'footer';
    return line === '' ? 'blank' : 'body';
  });
  for (const [index, line] of lines.entries()) {
    if (index === 0) {
      continue;
    }
    if (kinds[index] === 'trailer') {
      if (!line.startsWith('Co-authored-by: ')) {
        findings.push(`"${line.split(':')[0]}" must be spelled Co-authored-by`);
      }
      trailers.push(line.slice(line.indexOf(':') + 1).split('<')[0].trim());
    } else if (kinds[index] === 'footer') {
      const footer = line.match(ISSUE_FOOTER);
      if (!footer) {
        findings.push(`"${line}" is not a footer of the form Refs: #<n>`);
      } else if (footer[1] === 'Closes') {
        findings.push('a commit uses Refs, not Closes. The PR description closes the Issue');
      } else {
        footers.push({ key: footer[1], number: Number(footer[2]) });
      }
    } else if (kinds[index] === 'body' && line.length > MAX_LINE && !/https?:\/\/|`/.test(line)) {
      findings.push(`line ${index + 1} has ${line.length} characters, more than ${MAX_LINE}`);
    }
  }
  // The footers follow the body, and the trailers end the message. A blank line starts each block.
  for (const [kind, name, allowed] of [
    ['footer', 'Issue footers', ['footer', 'blank', 'trailer']],
    ['trailer', 'Co-authored-by trailers', ['trailer']],
  ]) {
    const first = kinds.indexOf(kind, 1);
    if (first === -1) {
      continue;
    }
    if (kinds[first - 1] !== 'blank') {
      findings.push(`put a blank line before the ${name}`);
    }
    if (kinds.slice(first).some((other) => !allowed.includes(other))) {
      findings.push(`the ${name} must come ${kind === 'footer' ? 'after the body' : 'at the end of the message'}`);
    }
  }
  const footerLines = kinds.flatMap((kind, index) => (kind === 'footer' ? [index] : []));
  if (footerLines.some((line, i) => i > 0 && line !== footerLines[i - 1] + 1)) {
    findings.push('the Issue footers are not on consecutive lines');
  }
  findings.push(...issueOrder(footers, 'footers'));
  const sorted = [...trailers].sort((a, b) => a.localeCompare(b));
  if (trailers.some((trailer, i) => trailer !== sorted[i])) {
    findings.push('the Co-authored-by trailers are not in alphabetical order');
  }
  return findings;
}

// Closes comes before Refs, and each group is in ascending Issue number.
function issueOrder(items, what) {
  const rank = (item) => [item.key === 'Closes' ? 0 : 1, item.number];
  for (let i = 1; i < items.length; i += 1) {
    const [a, b] = [rank(items[i - 1]), rank(items[i])];
    if (a[0] > b[0] || (a[0] === b[0] && a[1] >= b[1])) {
      return [`the Issue ${what} are not Closes then Refs, each in ascending Issue number`];
    }
  }
  return [];
}

export function checkLabels(labels, subject, rules, { migration }) {
  const findings = labels.filter((label) => !rules.labels.has(label)).map((label) => `label "${label}" is not in .github/labels.json`);
  const types = labels.filter((label) => label.startsWith('type:'));
  if (types.length !== 1 || types[0] !== `type:${subject.type}`) {
    findings.push(`apply exactly one type label, type:${subject.type}`);
  }
  if (subject.scope !== undefined) {
    const area = `area:${subject.scope}`;
    const areas = labels.filter((label) => label.startsWith('area:'));
    if (areas.length !== 1 || areas[0] !== area) {
      findings.push(`apply only the area label ${area}`);
    }
  }
  if (labels.includes('breaking') !== subject.breaking) {
    findings.push(subject.breaking ? 'apply breaking, because the title has !' : 'remove breaking, because the title has no !');
  }
  if (labels.includes('migration') !== migration) {
    findings.push(migration ? 'apply migration, because the diff changes a migration' : 'remove migration, because the diff changes no migration');
  }
  return findings;
}

// `breaking` and `revert` come from the PR title.
export function checkBody(body, { breaking = false, revert = false } = {}) {
  const lines = body.replace(/\r\n/g, '\n').split('\n');
  const findings = [];
  if (body.includes('<!--')) {
    findings.push('delete the HTML comments of the template');
  }
  let fence = '';
  const headings = [];
  for (const [index, line] of lines.entries()) {
    const marker = line.match(/^\s*(`{3,}|~{3,})/)?.[1]?.[0] ?? '';
    if (marker && (!fence || fence === marker)) {
      fence = fence ? '' : marker;
    } else if (!fence && line.startsWith('## ')) {
      headings.push({ title: line.slice(3).trim(), index });
    }
  }
  if (headings.map((heading) => heading.title).join('\n') !== PR_HEADINGS.join('\n')) {
    findings.push(`use the headings ${PR_HEADINGS.join(', ')}, in this order`);
    return findings;
  }
  // The non-blank lines of a section.
  const section = (title) => {
    const at = headings.findIndex((heading) => heading.title === title);
    return lines.slice(headings[at].index + 1, headings[at + 1]?.index ?? lines.length).filter((line) => line.trim() !== '');
  };
  for (const title of PR_HEADINGS) {
    if (section(title).length === 0) {
      findings.push(`section ${title} is empty; write n/a if there is nothing to report`);
    }
  }
  if (breaking && section('Breaking changes').map((line) => line.trim()).join('') === 'n/a') {
    findings.push('Breaking changes: the title has !, so state what breaks and what callers must change');
  }
  if (revert && !REVERT_LINE.test(section('What changed')[0] ?? '')) {
    findings.push('What changed: the title has the revert type, so start with "Reverts <full SHA>."');
  }
  const related = section('Related issues');
  const relatedNumbers = new Set();
  if (!(related.length === 1 && related[0] === 'n/a')) {
    const items = [];
    for (const line of related) {
      const match = line.match(/^(Closes|Refs) #(\d+)\.$/);
      if (match) {
        items.push({ key: match[1], number: Number(match[2]) });
        relatedNumbers.add(Number(match[2]));
      } else {
        findings.push(`Related issues: "${line}" is not "Closes #<n>." or "Refs #<n>."`);
      }
    }
    findings.push(...issueOrder(items, 'lines'));
  }
  // Follow-up tasks ends at the first text after a blank line that is not a bullet, such as
  // the attribution line of an agent harness.
  const followUp = [];
  let afterBlank = false;
  for (const line of lines.slice(headings.at(-1).index + 1)) {
    const item = /^(- |\s{2})/.test(line);
    if (line.trim() !== '' && !item && afterBlank && followUp.length > 0) {
      break;
    }
    afterBlank = line.trim() === '';
    if (!afterBlank) {
      followUp.push(line);
    }
  }
  if (!(followUp.length === 1 && followUp[0] === 'n/a')) {
    const bullets = followUp.filter((line) => line.startsWith('- '));
    if (!followUp[0]?.startsWith('- ') || followUp.some((line) => !/^(- |\s{2})/.test(line))) {
      findings.push('Follow-up tasks: write n/a or one bullet for each piece of work');
    }
    if (followUp.some((line) => /\b(closes|fixes|resolves)\s+#\d+/i.test(line))) {
      findings.push('Follow-up tasks: do not put Closes, Fixes, or Resolves before an Issue number');
    }
    // Bullets with an Issue come first, in ascending Issue number, then bullets without one.
    const numbers = bullets.map((line) => Number(line.match(/^- #(\d+): /)?.[1] ?? Infinity));
    if (numbers.some((number, i) => i > 0 && (number < numbers[i - 1] || (number === numbers[i - 1] && number !== Infinity)))) {
      findings.push('Follow-up tasks: put the bullets with an Issue first, in ascending Issue number');
    }
    const repeated = numbers.filter((number) => relatedNumbers.has(number));
    if (repeated.length > 0) {
      findings.push(`Follow-up tasks: do not repeat #${repeated.join(', #')} from Related issues`);
    }
  }
  return findings;
}

// Returns the prose units of a text: each paragraph, list item, or table row, without code,
// URLs, HTML comments, and headings. A commit message joins the hard-wrapped lines of each
// paragraph, so that a phrase across two lines counts. Markdown keeps a paragraph on one line.
function proseUnits(text, { commit }) {
  const lines = text.replace(/\r\n/g, '\n').replace(/<!--[\s\S]*?-->/g, '').split('\n');
  const units = [];
  let fence = '';
  let current;
  let currentTable = false;
  const end = () => {
    if (current) units.push(current);
    current = undefined;
  };
  for (const [index, raw] of lines.entries()) {
    const marker = raw.match(/^\s*(`{3,}|~{3,})/)?.[1] ?? '';
    if (marker && (!fence || (marker[0] === fence[0] && marker.length >= fence.length))) {
      fence = fence ? '' : marker;
      end();
      continue;
    }
    // A line indented by four spaces or a tab is an indented code block.
    if (fence || /^( {4}|\t)/.test(raw)) {
      end();
      continue;
    }
    if (commit && (index === 0 || ISSUE_KEYWORD.test(raw) || TRAILER_KEY.test(raw) || /^This reverts commit [0-9a-f]{7,40}\.$/.test(raw))) {
      end();
      continue;
    }
    const line = raw
      .replace(/(`+)[^`]*?\1/g, 'code')
      .replace(/!?\[([^\]]*)\]\([^)]*\)/g, '$1')
      .replace(/<https?:[^>]*>|https?:\/\/\S+?(?=[.,;:!?)]?(\s|$))/g, 'URL');
    if (line.trim() === '' || /^#{1,6}\s/.test(line)) {
      end();
      continue;
    }
    const item = /^\s*([-*+]|\d+\.)\s/.test(line);
    const table = /^\s*\|/.test(line);
    const textOnly = line.replace(/^\s*([-*+]|\d+\.)\s+/, '').trim();
    if (commit && current && !item && !table && !currentTable) {
      current += ` ${textOnly}`;
    } else {
      end();
      current = textOnly;
      currentTable = table;
    }
  }
  end();
  return units;
}

// Returns the findings for the rules of the simple-english skill that need no judgment.
export function checkProse(text, { commit = false } = {}) {
  const findings = [];
  for (const unit of proseUnits(text, { commit })) {
    for (const [pattern, message] of PROSE_RULES) {
      for (const match of unit.matchAll(pattern)) {
        findings.push(message(match[0]));
      }
    }
  }
  return findings;
}

function git(args) {
  const result = spawnSync('git', args, { encoding: 'utf8' });
  if (result.status !== 0) {
    throw new Error(`git ${args.join(' ')} failed: ${result.stderr.trim()}`);
  }
  return result.stdout;
}

export function run(options, rules, { git: runGit = git, read = (path) => readFileSync(path, 'utf8') } = {}) {
  const base = options.base ?? 'origin/main';
  const head = options.head ?? 'HEAD';
  const branch = options.branch ?? runGit(['rev-parse', '--abbrev-ref', 'HEAD']).trim();
  // Renovate writes its branch, commits, and description from its own configuration. Its PR
  // title becomes the subject of the squash commit, so the title and the labels follow the
  // conventions. Any author can name a branch renovate/, so only the PR author proves a
  // Renovate PR.
  const renovate = options.author === 'renovate[bot]' && branch.startsWith('renovate/');
  const findings = [];
  let title;
  if (options.title !== undefined) {
    title = checkSubject(options.title, rules);
    findings.push(...title.findings.map((finding) => `PR title: ${finding}`));
  }
  if (!renovate) {
    findings.push(...checkBranch(branch, title?.type).map((finding) => `Branch: ${finding}`));
    const log = runGit(['log', '--no-merges', '--reverse', '--format=%h%x00%B%x1e', `${base}..${head}`]);
    for (const entry of log.split('\x1e').map((text) => text.replace(/^\n/, '')).filter(Boolean)) {
      const [sha, message] = entry.split('\0');
      findings.push(...checkMessage(message, rules).map((finding) => `Commit ${sha}: ${finding}`));
      findings.push(...checkProse(message, { commit: true }).map((finding) => `Commit ${sha}: ${finding}`));
    }
  }
  if (options.labels !== undefined && title?.type) {
    const labels = options.labels.split(',').map((label) => label.trim()).filter(Boolean);
    const migration = runGit(['diff', '--name-only', `${base}...${head}`]).split('\n').some((path) => MIGRATION_PATH.test(path));
    findings.push(...checkLabels(labels, title, rules, { migration }).map((finding) => `Labels: ${finding}`));
  }
  if (options['body-file'] !== undefined && !renovate) {
    const body = read(options['body-file']);
    const fromTitle = { breaking: title?.breaking ?? false, revert: title?.type === 'revert' };
    findings.push(...checkBody(body, fromTitle).map((finding) => `PR description: ${finding}`));
    findings.push(...checkProse(body).map((finding) => `PR description: ${finding}`));
  }
  return findings;
}

function main() {
  const { values } = parseArgs({
    options: Object.fromEntries(
      ['base', 'head', 'branch', 'author', 'title', 'labels', 'body-file'].map((name) => [name, { type: 'string' }]),
    ),
  });
  const root = fileURLToPath(new URL('..', import.meta.url));
  const findings = run(values, loadRules(root));
  // The findings quote PR metadata that any author controls. In GitHub Actions, stop the
  // runner from reading workflow commands in them, such as ::error:: or ##[warning].
  const token = process.env.GITHUB_ACTIONS === 'true' ? randomUUID() : undefined;
  if (token) {
    console.log(`::stop-commands::${token}`);
  }
  for (const finding of findings) {
    console.log(finding);
  }
  if (token) {
    console.log(`::${token}::`);
  }
  if (findings.length > 0) {
    process.exit(1);
  }
  console.log('PR metadata follows the checked convention rules.');
}

if (process.argv[1] === fileURLToPath(import.meta.url)) {
  main();
}
