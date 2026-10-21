#!/usr/bin/env node
// Checks the prose that a branch adds to Markdown files and code comments against the rules
// of the Markdown and English prose convention that need no judgment, so that writing-reviewer
// can skip them. check-pr-metadata.mjs owns the prose rules of the simple-english skill, and
// this script applies the same rules. It checks these rules:
//
// - In Markdown files, each paragraph or list item that has an added line, outside code and
//   HTML comments: the prose rules, no bold or italic text, straight quotation marks, no bullet
//   list inside a list, and no paragraph or list item that continues on a new line.
// - In code comments, each comment paragraph that has an added line: the prose rules and
//   straight quotation marks. The script reads only comments on their own lines, and it skips
//   tool directives, such as //go:build or -- +goose Up, indented code, raw strings, and YAML
//   block scalars.
//
// It skips the paths that the convention excludes, test data, generated files, the status
// line of an ADR, and the Status column of the ADR index, because the ADR convention fixes
// their values, such as `Accepted; scoped exception to ADR-0011`.
//
// Without --head, it checks the working tree, so that it sees changes before a commit.
//
//   node scripts/checks/check-prose.mjs [--base <ref>] [--head <ref>]

import { spawnSync } from 'node:child_process';
import { randomUUID } from 'node:crypto';
import { readFileSync } from 'node:fs';
import { basename } from 'node:path';
import { fileURLToPath } from 'node:url';
import { parseArgs } from 'node:util';

import { checkProse } from './check-pr-metadata.mjs';

const EXCLUDED = /^(\.agents|\.claude)\/|(^|\/)(gen|sqlc|testdata)\/|\.pb\.go$/;
const MARKERS = { '.go': '//', '.proto': '//', '.js': '//', '.mjs': '//', '.sql': '--', '.sh': '#', '.yaml': '#', '.yml': '#', '.toml': '#' };
const HASH_FILES = new Set(['Tiltfile', 'Dockerfile']);
const FENCE = /^\s*(`{3,}|~{3,})/;
const GENERATED = /^\/\/ Code generated .* DO NOT EDIT\.$/m;
// A tool reads these comments, such as -- +goose Up or -- name: GetTask :one in sqlc queries.
const DIRECTIVE = /^\s*(\+\w|name:\s)/;
const CURLY = /[‘’“”]/g;
const EMPHASIS = /\*\*[^*\s][^*]*\*\*|__[^_\s][^_]*__|(^|[^\w*])\*[^*\s][^*]*?\*(?![\w*])|(^|[^\w_])_[^_\s][^_]*?_(?![\w_])/;

const MESSAGES = {
  emphasis: 'bold or italic text. Write plain text',
  curly: 'a curly quotation mark. Use a straight one',
  nested: 'a bullet list inside a list. Only a numbered list can go inside a bullet',
  wrapped: 'a paragraph continues on a new line. Keep each paragraph and list item on 1 line',
};

const markerOf = (path) => (HASH_FILES.has(basename(path)) ? '#' : MARKERS[path.slice(path.lastIndexOf('.'))]);

export function inScope(path) {
  return !EXCLUDED.test(path) && (path.endsWith('.md') || markerOf(path) !== undefined);
}

// Returns the added line numbers of each file in a diff that `git diff --unified=0` prints.
export function addedLines(diff) {
  const files = new Map();
  let current;
  for (const line of diff.split('\n')) {
    if (line.startsWith('+++ ')) {
      current = line.startsWith('+++ b/') ? new Set() : undefined;
      if (current) files.set(line.slice('+++ b/'.length), current);
      continue;
    }
    const hunk = line.match(/^@@ -\S+ \+(\d+)(?:,(\d+))? @@/);
    if (hunk && current) {
      const start = Number(hunk[1]);
      for (let n = start; n < start + Number(hunk[2] ?? 1); n += 1) current.add(n);
    }
  }
  return files;
}

const curly = (text) => [...text.matchAll(CURLY)].map(() => MESSAGES.curly);

function kind(line, inList) {
  if (line.trim() === '') return 'blank';
  if (/^\s*([-*+]|\d+\.)\s/.test(line)) return 'list';
  if (/^#{1,6}\s/.test(line)) return 'heading';
  if (/^\s*[|><]/.test(line)) return 'other';
  // Inside a list, an indented line belongs to a list item. markdownlint (MD046) allows only
  // fenced code blocks, so an indented line there is never code.
  if (/^( {4}|\t)/.test(line) && !inList) return 'code';
  return 'text';
}

// Returns the type and the prose block of each line. A line that is not prose, such as code or
// an HTML comment, has no block. A paragraph or a list item is 1 block, and a text line that
// follows it continues it. Each heading, table row, and other line is a block of its own.
function proseBlocks(lines) {
  const types = [];
  const blocks = [];
  let fence = '';
  let comment = false;
  let inList = false;
  let block = -1;
  let open = false;
  for (const line of lines) {
    const marker = line.match(FENCE)?.[1] ?? '';
    const isFence = marker && (!fence || (marker[0] === fence[0] && marker.length >= fence.length));
    if (isFence) fence = fence ? '' : marker;
    const isComment = !isFence && !fence && (comment || line.trimStart().startsWith('<!--'));
    if (isComment) comment = !line.includes('-->');
    const type = isFence || fence || isComment ? 'none' : kind(line, inList);
    if (type === 'list') inList = true;
    else if (['heading', 'other'].includes(type) || (type === 'text' && !/^\s/.test(line))) inList = false;
    const own = ['heading', 'other'].includes(type);
    if (type === 'list' || own || (type === 'text' && !open)) block += 1;
    open = type === 'list' || type === 'text';
    types.push(type);
    blocks.push(open || own ? block : undefined);
  }
  return { types, blocks };
}

export function checkMarkdown(text, added) {
  const findings = [];
  const lines = text.split('\n');
  const { types, blocks } = proseBlocks(lines);
  // The convention applies to the whole paragraph or list item that a change edits.
  const touched = new Set([...added].map((n) => blocks[n - 1]).filter((block) => block !== undefined));
  for (const [index, line] of lines.entries()) {
    if (!touched.has(blocks[index])) continue;
    const plain = line
      .replace(/<!--.*?-->/g, '')
      .replace(/(`+)[^`]*?\1/g, 'code')
      .replace(/\]\([^)]*\)/g, ']')
      .replace(/<https?:[^>]*>|https?:\/\/\S+/g, 'URL');
    const messages = [...checkProse(line.trimStart())];
    if (EMPHASIS.test(plain)) messages.push(MESSAGES.emphasis);
    messages.push(...curly(plain));
    if (/^\s+[-*+]\s/.test(line)) messages.push(MESSAGES.nested);
    if (types[index] === 'text' && blocks[index - 1] === blocks[index]) messages.push(MESSAGES.wrapped);
    findings.push(...messages.map((message) => ({ line: index + 1, message })));
  }
  return findings;
}

export function checkComments(path, text, added) {
  const marker = markerOf(path);
  if (!marker) return [];
  const findings = [];
  let block;
  const end = () => {
    if (block && block.lines.some((n) => added.has(n))) {
      const prose = block.text.join(' ');
      findings.push(...[...checkProse(prose), ...curly(prose)].map((message) => ({ line: block.lines[0], message })));
    }
    block = undefined;
  };
  // Go and JavaScript keep a raw string between backticks, and a line inside it is not a comment.
  const raw = ['.go', '.js', '.mjs'].some((ext) => path.endsWith(ext));
  let inRaw = false;
  // A YAML block scalar, such as `key: |`, holds a string value on its more indented lines.
  const yaml = /\.ya?ml$/.test(path);
  let scalar = -1;
  for (const [index, line] of text.split('\n').entries()) {
    const trimmed = line.trimStart();
    const indent = line.length - trimmed.length;
    if (scalar >= 0 && (trimmed === '' || indent > scalar)) {
      end();
      continue;
    }
    scalar = -1;
    if (inRaw) {
      inRaw = (line.match(/`/g) ?? []).length % 2 === 0;
      end();
      continue;
    }
    const content = trimmed.startsWith(marker) ? trimmed.slice(marker.length) : undefined;
    // A comment on its own line has a space after the marker. A directive, such as //go:build,
    // or a shebang has none, and an indented line inside a comment is code.
    if (content === undefined || content.trim() === '' || !content.startsWith(' ') || DIRECTIVE.test(content) || /^ ?(\t| {4})/.test(content)) {
      if (raw && content === undefined) {
        // Strings on 1 line and a trailing comment cannot open a raw string.
        const code = line.replace(/"(\\.|[^"\\])*"|'(\\.|[^'\\])*'|`[^`]*`/g, '').replace(/\/\/.*$/, '');
        inRaw = code.includes('`');
      }
      if (yaml && content === undefined && /(^-|:)\s*[|>][-+0-9]*\s*(#.*)?$/.test(trimmed)) scalar = indent;
      end();
      continue;
    }
    block ??= { lines: [], text: [] };
    block.lines.push(index + 1);
    block.text.push(content.trim());
  }
  end();
  return findings;
}

function git(args) {
  const result = spawnSync('git', args, { encoding: 'utf8', maxBuffer: 64 * 1024 * 1024 });
  if (result.status !== 0) {
    throw new Error(`git ${args.join(' ')} failed: ${result.stderr.trim()}`);
  }
  return result.stdout;
}

// With a head, run checks the files of that commit, as CI does. Without one, it checks the
// working tree against the merge base, with the staged, unstaged, and new files.
export function run(options, { git: runGit = git, read = (path) => readFileSync(path, 'utf8') } = {}) {
  const base = options.base ?? 'origin/main';
  const head = options.head;
  const diffArgs = ['diff', '--unified=0', '--no-color', '--no-ext-diff', '--diff-filter=AMR'];
  let files;
  let readFile = read;
  if (head) {
    files = addedLines(runGit([...diffArgs, `${base}...${head}`]));
    readFile = (path) => runGit(['show', `${head}:${path}`]);
  } else {
    files = addedLines(runGit([...diffArgs, runGit(['merge-base', base, 'HEAD']).trim()]));
    // A new file has no diff yet, so each of its lines counts as added.
    for (const path of runGit(['ls-files', '--others', '--exclude-standard', '-z']).split('\0').filter(Boolean)) {
      files.set(path, undefined);
    }
  }
  const findings = [];
  for (let [path, added] of files) {
    if (added?.size === 0 || !inScope(path)) continue;
    let text = readFile(path);
    added ??= new Set(text.split('\n').map((_, index) => index + 1));
    if (GENERATED.test(text)) continue;
    if (path === 'docs/adr/README.md') {
      // The last column of the index copies the status line of each ADR.
      text = text.replace(/^(\|.*\|)[^|\n]*\|[ \t]*$/gm, '$1 |');
    } else if (path.startsWith('docs/adr/')) {
      text.split('\n').forEach((line, index) => line.startsWith('Status: ') && added.delete(index + 1));
    }
    const found = path.endsWith('.md') ? checkMarkdown(text, added) : checkComments(path, text, added);
    findings.push(...found.map(({ line, message }) => `${path}:${line}: ${message}`));
  }
  return findings;
}

function main() {
  const { values } = parseArgs({ options: { base: { type: 'string' }, head: { type: 'string' } } });
  const findings = run(values);
  // The findings quote text that any PR author controls. In GitHub Actions, stop the runner
  // from reading workflow commands in them, such as ::error::.
  const token = process.env.GITHUB_ACTIONS === 'true' ? randomUUID() : undefined;
  if (token) console.log(`::stop-commands::${token}`);
  for (const finding of findings) console.log(finding);
  if (token) console.log(`::${token}::`);
  if (findings.length > 0) process.exit(1);
  console.log('The added prose follows the checked convention rules.');
}

if (process.argv[1] === fileURLToPath(import.meta.url)) {
  main();
}
