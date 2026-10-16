import assert from 'node:assert/strict';
import test from 'node:test';

import { addedLines, checkComments, checkMarkdown, inScope, run } from './check-prose.mjs';

const SHOULD = '"should" is a modal that simple-english forbids. Use can, will, or must';
const EMPHASIS = 'bold or italic text. Write plain text';
const CURLY = 'a curly quotation mark. Use a straight one';
const NESTED = 'a bullet list inside a list. Only a numbered list can go inside a bullet';
const WRAPPED = 'a paragraph continues on a new line. Keep each paragraph and list item on 1 line';

const lines = (...numbers) => new Set(numbers);

test('addedLines reads the added line numbers of each file from a diff without context', () => {
  const diff = [
    'diff --git a/docs/a.md b/docs/a.md',
    '+++ b/docs/a.md',
    '@@ -3,0 +4,2 @@ heading',
    '+one',
    '+two',
    '@@ -9 +11 @@',
    '-old',
    '+new',
    'diff --git a/docs/b.md b/docs/b.md',
    '+++ b/docs/b.md',
    '@@ -5,2 +4,0 @@',
    '-gone',
    '-gone',
    'diff --git a/docs/c.md b/docs/c.md',
    '+++ /dev/null',
  ].join('\n');
  assert.deepEqual(addedLines(diff), new Map([['docs/a.md', lines(4, 5, 11)], ['docs/b.md', lines()]]));
});

test('inScope skips the paths that the prose convention excludes and generated code', () => {
  for (const path of ['docs/a.md', 'README.md', 'services/work/internal/app/task.go', 'contracts/proto/work/v1/work.proto', 'deploy/base/x.yaml', 'Tiltfile']) {
    assert.equal(inScope(path), true, path);
  }
  for (const path of ['.agents/agents/x.md', '.claude/rules/x.md', 'gen/go/work/v1/work.pb.go', 'services/work/internal/adapters/sqlc/db.go', 'x/y.pb.go', 'internal/x/testdata/a.md', 'go.sum']) {
    assert.equal(inScope(path), false, path);
  }
});

test('checkMarkdown checks only the added lines', () => {
  const text = 'It should run.\n\nIt should stop.\n';
  assert.deepEqual(checkMarkdown(text, lines(3)), [{ line: 3, message: SHOULD }]);
  assert.deepEqual(checkMarkdown(text, lines()), []);
});

test('checkMarkdown skips added lines inside a code block that an unchanged line opens', () => {
  const text = '```text\nIt should run.\n```\n\nIt should stop.\n';
  assert.deepEqual(checkMarkdown(text, lines(2, 5)), [{ line: 5, message: SHOULD }]);
});

test('checkMarkdown finds bold and italic text outside code', () => {
  assert.deepEqual(checkMarkdown('Run **all** checks.\n', lines(1)), [{ line: 1, message: EMPHASIS }]);
  assert.deepEqual(checkMarkdown('Run __all__ checks.\n', lines(1)), [{ line: 1, message: EMPHASIS }]);
  assert.deepEqual(checkMarkdown('Run *all* checks.\n', lines(1)), [{ line: 1, message: EMPHASIS }]);
  assert.deepEqual(checkMarkdown('Run _all_ checks.\n', lines(1)), [{ line: 1, message: EMPHASIS }]);
  assert.deepEqual(checkMarkdown('* Run `**/*.md` and `a_b_c` with snake_case_names, 2 * 3 * 4.\n', lines(1)), []);
});

test('checkMarkdown finds curly quotation marks', () => {
  assert.deepEqual(checkMarkdown('Write “done”.\n', lines(1)), [{ line: 1, message: CURLY }, { line: 1, message: CURLY }]);
  assert.deepEqual(checkMarkdown("Write 'done' and \"done\".\n", lines(1)), []);
});

test('checkMarkdown finds a bullet list inside a list and allows a numbered list inside a bullet', () => {
  const text = '- Run it.\n  - Then stop.\n- Do this:\n  1. Run it.\n  2. Stop.\n';
  assert.deepEqual(checkMarkdown(text, lines(1, 2, 3, 4, 5)), [{ line: 2, message: NESTED }]);
});

test('checkMarkdown finds a paragraph or a list item that continues on a new line', () => {
  assert.deepEqual(checkMarkdown('It runs\nin order.\n', lines(2)), [{ line: 2, message: WRAPPED }]);
  assert.deepEqual(checkMarkdown('- It runs\n  in order.\n', lines(2)), [{ line: 2, message: WRAPPED }]);
  const valid = '# Title\nIt runs.\n\n- One.\n- Two.\n1. One.\n\n| a | b |\n| --- | --- |\n| c | d |\n\n> Note.\n> More.\n\n- Do this:\n\n  The step runs.\n<!-- note -->\n';
  assert.deepEqual(checkMarkdown(valid, lines(...valid.split('\n').map((_, i) => i + 1))), []);
});

test('run without a head checks the working tree, including staged, unstaged, and new files', () => {
  const files = { 'docs/a.md': 'It should run.\n', 'docs/new.md': 'It may run.\n' };
  const calls = [];
  const git = (args) => {
    calls.push(args.join(' '));
    if (args[0] === 'merge-base') return 'mb\n';
    if (args[0] === 'diff') return 'diff --git a/docs/a.md b/docs/a.md\n+++ b/docs/a.md\n@@ -0,0 +1 @@\n';
    if (args[0] === 'ls-files') return 'docs/new.md\0.agents/x.md\0';
    throw new Error(`unexpected git ${args.join(' ')}`);
  };
  const read = (path) => files[path];
  assert.deepEqual(run({ base: 'origin/main' }, { git, read }), [
    'docs/a.md:1: ' + SHOULD,
    'docs/new.md:1: "may" is a modal that simple-english forbids. Use can, will, or must',
  ]);
  assert.deepEqual(calls, [
    'merge-base origin/main HEAD',
    'diff --unified=0 --no-color --no-ext-diff --diff-filter=AMR mb',
    'ls-files --others --exclude-standard -z',
  ]);
});

test('run skips the status line of an ADR, whose values the ADR convention fixes', () => {
  const text = '# ADR-0050: Example\n\nDate: 2026-10-15\n\nStatus: Accepted; scoped exception to ADR-0011\n\nIt runs; it stops.\n';
  const git = (args) => (args[0] === 'diff' ? 'diff --git a/docs/adr/0050-example.md b/docs/adr/0050-example.md\n+++ b/docs/adr/0050-example.md\n@@ -0,0 +1,7 @@\n' : text);
  assert.deepEqual(run({ head: 'head' }, { git }), ['docs/adr/0050-example.md:7: a semicolon joins two sentences. Write two sentences']);
});

test('run skips the Status column of the ADR index and checks the Decision column', () => {
  const text = '| # | Decision | Status |\n| --- | --- | --- |\n| [0050](0050-a.md) | It runs | Accepted; scoped exception to [0011](0011-b.md) |\n| [0051](0051-c.md) | It runs; it stops | Accepted |\n';
  const git = (args) => (args[0] === 'diff' ? 'diff --git a/docs/adr/README.md b/docs/adr/README.md\n+++ b/docs/adr/README.md\n@@ -0,0 +3,2 @@\n' : text);
  assert.deepEqual(run({ head: 'head' }, { git }), ['docs/adr/README.md:4: a semicolon joins two sentences. Write two sentences']);
});

test('checkMarkdown skips indented code and HTML comments across lines', () => {
  const text = 'Run it:\n\n    x := "**bold**"; y should be 1\n\n<!--\nIt should run.\nIt runs.\n-->\nIt runs.\n';
  assert.deepEqual(checkMarkdown(text, lines(3, 5, 6, 7, 8, 9)), []);
});

test('checkMarkdown checks each table row and quote line on its own', () => {
  const text = '| a | b |\n| --- | --- |\n| It should run | yes |\n| It should stop | no |\n\n> It should run.\n';
  assert.deepEqual(checkMarkdown(text, lines(3, 6)), [{ line: 3, message: SHOULD }, { line: 6, message: SHOULD }]);
});

test('checkMarkdown checks the format of headings', () => {
  assert.deepEqual(checkMarkdown('# **Important** “note”\n', lines(1)), [
    { line: 1, message: EMPHASIS },
    { line: 1, message: CURLY },
    { line: 1, message: CURLY },
  ]);
});

test('checkMarkdown reads a list item indented by 4 spaces as a list item, not as code', () => {
  assert.deepEqual(checkMarkdown('- Parent.\n    - Child.\n', lines(2)), [{ line: 2, message: NESTED }]);
  assert.deepEqual(checkMarkdown('- Steps:\n\n    1. It should run.\n', lines(3)), [{ line: 3, message: SHOULD }]);
});

test('checkMarkdown checks the whole paragraph or list item when 1 of its lines changes', () => {
  assert.deepEqual(checkMarkdown('Updated paragraph\ncontinues here.\n', lines(1)), [{ line: 2, message: WRAPPED }]);
  assert.deepEqual(checkMarkdown('- Updated item\n  should continue.\n- Other item should stay.\n', lines(1)), [
    { line: 2, message: SHOULD },
    { line: 2, message: WRAPPED },
  ]);
});

test('checkComments checks each comment paragraph that has an added line, across its lines', () => {
  const text = [
    'package x',
    '',
    '// Run starts the job. It has',
    '// been safe to call twice.',
    '//',
    '// It should stop.',
    'func Run() {}',
    '',
    '// Other should stay unchecked.',
    'func Other() {}',
  ].join('\n');
  assert.deepEqual(checkComments('x.go', text, lines(3, 7)), [{ line: 3, message: '"has been" is the present perfect. Use a simple tense' }]);
  assert.deepEqual(checkComments('x.go', text, lines(6)), [{ line: 6, message: SHOULD }]);
});

test('checkComments skips directives, indented code, and files that it does not know', () => {
  const text = '//go:build integration\n\n// Example:\n//\n//\tx should be 1\n';
  assert.deepEqual(checkComments('x.go', text, lines(1, 3, 4, 5)), []);
  assert.deepEqual(checkComments('x.png', '// It should run.\n', lines(1)), []);
});

test('checkComments skips comment markers inside a Go or JavaScript raw string', () => {
  const text = 'const text = `\n// It should run.\n`\n\n// It runs.\n';
  assert.deepEqual(checkComments('x.go', text, lines(1, 2, 3, 5)), []);
  assert.deepEqual(checkComments('x.mjs', text, lines(1, 2, 3, 5)), []);
  assert.deepEqual(checkComments('x.go', 'x := "a`b"\n// It should run.\n', lines(2)), [{ line: 2, message: SHOULD }]);
});

test('checkComments skips lines inside a YAML block scalar, which hold a string value', () => {
  const text = 'message: |\n  # It should run.\n  text\n# It should stop.\nlist:\n  - >-\n    # It may run.\n';
  assert.deepEqual(checkComments('a.yaml', text, lines(1, 2, 3, 4, 5, 6, 7)), [{ line: 4, message: SHOULD }]);
});

test('checkComments ignores a backtick in a trailing comment', () => {
  const text = 'var x = 1 // A backtick is `\n// It should run.\ny := `http://a` // done\n// It should stop.\n';
  assert.deepEqual(checkComments('x.go', text, lines(2, 4)), [{ line: 2, message: SHOULD }, { line: 4, message: SHOULD }]);
});

test('checkComments reads the comment marker of each kind of file', () => {
  assert.deepEqual(checkComments('a.sql', '-- +goose Up\n-- It should run.\nSELECT 1;\n', lines(2)), [{ line: 2, message: SHOULD }]);
  assert.deepEqual(checkComments('a.yaml', '#!/bin/sh\n# It should run.\nkey: value # It should not count.\n', lines(1, 2, 3)), [{ line: 2, message: SHOULD }]);
  assert.deepEqual(checkComments('Tiltfile', '# It should run.\n', lines(1)), [{ line: 1, message: SHOULD }]);
  assert.deepEqual(checkComments('a.proto', '// It should run.\n', lines(1)), [{ line: 1, message: SHOULD }]);
  assert.deepEqual(checkComments('a.go', '// Write “done”.\n', lines(1)), [{ line: 1, message: CURLY }, { line: 1, message: CURLY }]);
});

test('run reads the branch diff and the files at the head, and skips generated files', () => {
  const files = {
    'docs/a.md': 'It should run.\n',
    'x.go': '// Code generated by a tool. DO NOT EDIT.\n\n// It should run.\n',
    '.agents/x.md': 'It should run.\n',
  };
  const calls = [];
  const git = (args) => {
    calls.push(args.join(' '));
    if (args[0] === 'diff') {
      return Object.keys(files).map((path) => `diff --git a/${path} b/${path}\n+++ b/${path}\n@@ -0,0 +1,3 @@\n`).join('');
    }
    return files[args[1].slice('head:'.length)];
  };
  assert.deepEqual(run({ base: 'base', head: 'head' }, { git }), ['docs/a.md:1: ' + SHOULD]);
  assert.equal(calls[0], 'diff --unified=0 --no-color --no-ext-diff --diff-filter=AMR base...head');
});
