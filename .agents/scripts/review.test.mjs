import assert from 'node:assert/strict';
import { spawnSync } from 'node:child_process';
import { createHash } from 'node:crypto';
import { existsSync, lstatSync, mkdirSync, mkdtempSync, readFileSync, readdirSync, realpathSync, rmSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { dirname, join } from 'node:path';
import test from 'node:test';
import { fileURLToPath } from 'node:url';

import {
  CONFIG_PATH,
  HOME_SECRETS,
  WORKSPACE_SECRETS,
  isApproved,
  loadRunner,
  promptWithSnapshot,
  roleRunner,
  roleSettings,
  runnerPath,
  stagedTree,
  stampPath,
  textStampPath,
} from './review.mjs';
import * as claude from './review-runners/claude.mjs';
import { args as codexArgs, sandboxArgs, sessionID } from './review-runners/codex.mjs';

const ROOT = join(dirname(fileURLToPath(import.meta.url)), '..', '..');
const SCRIPT = join(ROOT, '.agents/scripts/review.mjs');
const ROLES = ['code-reviewer', 'contract-reviewer', 'convention-reviewer', 'infra-reviewer', 'migration-reviewer',
  'planning-reviewer', 'security-auditor', 'spec-conformance-reviewer', 'test-reviewer', 'writing-reviewer'];

test('the role configuration gives every role a runner that loads and accepts its effort', async () => {
  const config = readFileSync(join(ROOT, CONFIG_PATH), 'utf8');
  assert.deepEqual(Object.keys(JSON.parse(config)).sort(), ROLES);
  for (const role of ROLES) {
    await roleRunner(ROOT, roleSettings(config, role));
  }
});

test('every role has its instructions in .agents/agents/ and no agent-specific wrapper', () => {
  for (const role of ROLES) {
    assert.ok(readFileSync(join(ROOT, '.agents/agents', `${role}.md`), 'utf8').length > 0, role);
  }
  assert.equal(existsSync(join(ROOT, '.claude/agents')), false);
  assert.equal(existsSync(join(ROOT, '.codex/agents')), false);
});

test('Claude Code and AGENTS.md reach the same review roles rules', () => {
  const rules = join(ROOT, '.agents/rules/review-roles.md');
  const link = join(ROOT, '.claude/rules/review-roles.md');
  assert.ok(lstatSync(link).isSymbolicLink());
  assert.equal(realpathSync(link), realpathSync(rules));
  assert.ok(readFileSync(join(ROOT, 'AGENTS.md'), 'utf8').includes('(.agents/rules/review-roles.md)'));
  assert.equal(existsSync(join(ROOT, '.claude/rules/codex-review-roles.md')), false);
});

test('roleRunner rejects a runner that does not exist and an effort that the runner does not accept', async () => {
  // The repository can gain runners, so this test makes sure that the existing runners appear in the
  // list. The test with the fixture repository below makes sure that the list contains exactly its runners.
  const missing = /names the runner no-such-runner, but \.agents\/scripts\/review-runners\/ has only: (.*)$/;
  await assert.rejects(roleRunner(ROOT, { runner: 'no-such-runner', model: 'm', effort: 'high' }), (error) => {
    const names = error.message.match(missing)?.[1].split(', ') ?? [];
    return names.includes('claude') && names.includes('codex');
  });
  await assert.rejects(roleRunner(ROOT, { runner: 'claude', model: 'm', effort: 'ultra' }), /the claude runner does not accept the effort ultra\. Use one of: low, medium, high, xhigh, max/);
  await assert.rejects(roleRunner(ROOT, { runner: 'codex', model: 'm', effort: 'minimal' }), /the codex runner does not accept the effort minimal/);
  assert.equal((await roleRunner(ROOT, { runner: 'codex', model: 'm', effort: 'ultra' })).command, 'codex');
  assert.equal((await roleRunner(ROOT, { runner: 'claude', model: 'm', effort: 'max' })).command, 'claude');
});

test('roleSettings reads one role and rejects an incomplete or unknown role', () => {
  const config = JSON.stringify({
    'code-reviewer': { runner: 'codex', model: 'm1', effort: 'high' },
    'writing-reviewer': { runner: 'claude', model: 'm2', effort: 'low', extra: 'ignored' },
  });
  assert.deepEqual(roleSettings(config, 'code-reviewer'), { runner: 'codex', model: 'm1', effort: 'high' });
  assert.deepEqual(roleSettings(config, 'writing-reviewer'), { runner: 'claude', model: 'm2', effort: 'low' });
  assert.throws(() => roleSettings(config, 'test-engineer'), /no role test-engineer/);
  assert.throws(() => roleSettings(config, 'constructor'), /no role constructor/);
  assert.throws(() => roleSettings('{', 'code-reviewer'), /not valid JSON/);
  assert.throws(() => roleSettings('null', 'code-reviewer'), /no role/);
  for (const key of ['runner', 'model', 'effort']) {
    const settings = { runner: 'codex', model: 'm', effort: 'high' };
    for (const value of [undefined, '', 3]) {
      const role = { ...settings, [key]: value };
      assert.throws(() => roleSettings(JSON.stringify({ r: role }), 'r'), new RegExp(`needs a ${key}`), `${key}=${value}`);
    }
  }
});

test('runnerPath keeps the runner inside the runners directory', () => {
  assert.equal(runnerPath('/repo', 'codex'), '/repo/.agents/scripts/review-runners/codex.mjs');
  assert.equal(runnerPath('/repo', 'gemini-cli'), '/repo/.agents/scripts/review-runners/gemini-cli.mjs');
  for (const name of ['../codex', 'a/b', 'Codex', '', '.hidden', 'codex.mjs']) {
    assert.throws(() => runnerPath('/repo', name), /runner name/, name);
  }
});

test('loadRunner rejects a missing module and a module without the runner exports', async () => {
  const dir = mkdtempSync(join(tmpdir(), 'review-runner-'));
  try {
    await assert.rejects(loadRunner(join(dir, 'missing.mjs')), /cannot load runner/);
    writeFileSync(join(dir, 'partial.mjs'), "export const command = 'x';\nexport function args() { return []; }\n");
    await assert.rejects(loadRunner(join(dir, 'partial.mjs')), /must export command, args\(\), and sessionID\(\)/);
    writeFileSync(join(dir, 'bad-report.mjs'), "export const command = 'x';\nexport const args = () => [];\nexport const sessionID = () => 'id';\nexport const report = 'r';\n");
    await assert.rejects(loadRunner(join(dir, 'bad-report.mjs')), /must export efforts/);
    writeFileSync(join(dir, 'report.mjs'), "export const command = 'x';\nexport const efforts = ['high'];\nexport const args = () => [];\nexport const sessionID = () => 'id';\nexport const report = 'r';\n");
    await assert.rejects(loadRunner(join(dir, 'report.mjs')), /report, but it is not a function/);
    for (const [i, efforts] of ['[]', "'high'", '[1]'].entries()) {
      writeFileSync(join(dir, `efforts-${i}.mjs`), `export const command = 'x';\nexport const efforts = ${efforts};\nexport const args = () => [];\nexport const sessionID = () => 'id';\n`);
      await assert.rejects(loadRunner(join(dir, `efforts-${i}.mjs`)), /must export efforts/, efforts);
    }
  } finally {
    rmSync(dir, { recursive: true, force: true });
  }
});

test('stamps live under reviews/ in the common Git directory', () => {
  assert.equal(stampPath('/repo/.git', 'code-reviewer', 't1'), '/repo/.git/reviews/code-reviewer/t1');
  const hash = createHash('sha256').update('text\n').digest('hex');
  assert.equal(textStampPath('/repo/.git', 'writing-reviewer', 'text\n'), `/repo/.git/reviews/writing-reviewer/text-${hash}`);
  assert.notEqual(textStampPath('/g', 'r', 'text\n'), textStampPath('/g', 'r', 'text'));
});

test('review helpers read verdicts, and the Codex runner reads session IDs', () => {
  assert.equal(isApproved('## Review\n\n**Verdict:** APPROVE\n'), true);
  assert.equal(isApproved('**Verdict: APPROVE**'), true);
  assert.equal(isApproved('**Verdict:** APPROVE | REQUEST CHANGES'), false);
  assert.equal(isApproved('**Verdict: REQUEST CHANGES**'), false);
  const rejectedAfterApproval = '**Verdict:** REQUEST CHANGES\n\nThe last round said **Verdict:** APPROVE, but a new bug appeared.\n';
  assert.equal(isApproved(rejectedAfterApproval), false);
  assert.equal(isApproved('**Verdict:** REQUEST CHANGES\n\n> **Verdict:** APPROVE\n'), false);
  assert.equal(isApproved('**Verdict:** APPROVE\n\n**Verdict:** REQUEST CHANGES\n'), false);
  assert.equal(isApproved('Verdict: APPROVE\n\nThis resolves the earlier REQUEST CHANGES verdict.\n'), true);
  assert.equal(sessionID('model: x\nsession id: 01a1-22\n'), '01a1-22');
});

test('an empty report or a repeated APPROVE verdict does not approve', () => {
  assert.equal(isApproved(''), false);
  assert.equal(isApproved('**Verdict:** APPROVE\n\n**Verdict:** APPROVE\n'), false);
});

test('the review prompt carries the exact tree and description that the stamps cover', () => {
  const text = 'Description under review\n';
  const prompt = promptWithSnapshot('Review this.', { tree: 't1', head: 'h1', text });
  assert.match(prompt, /git diff h1 t1/);
  assert.ok(prompt.includes(text));
  assert.ok(prompt.includes(createHash('sha256').update(text).digest('hex')));
  assert.equal(textStampPath('/g', 'r', text), stampPath('/g', 'r', `text-${createHash('sha256').update(text).digest('hex')}`));
  assert.ok(!promptWithSnapshot('Review this.', { tree: 't1', head: 'h1' }).includes('<pr-description>'));
  const committed = promptWithSnapshot('Review git diff origin/main...HEAD.', { tree: 't1', head: 'h1', headTree: 't1' });
  assert.ok(!committed.includes('git diff h1 t1'));
  assert.match(committed, /Nothing is staged beyond HEAD/);
});

function gitRepo(prefix) {
  const repo = mkdtempSync(join(tmpdir(), prefix));
  const git = (...args) => spawnSync('git', ['-c', 'user.name=t', '-c', 'user.email=t@example.com', ...args], { cwd: repo, encoding: 'utf8' });
  git('init', '-q');
  return { repo, git };
}

test('stagedTree works while another process holds index.lock', () => {
  const repo = mkdtempSync(join(tmpdir(), 'git-guard-lock-'));
  const git = (...args) => spawnSync('git', ['-c', 'user.name=t', '-c', 'user.email=t@example.com', ...args], { cwd: repo, encoding: 'utf8' });
  try {
    git('init', '-q');
    writeFileSync(join(repo, 'a.txt'), 'staged\n');
    git('add', 'a.txt');
    const expected = git('write-tree').stdout.trim();
    writeFileSync(join(repo, '.git', 'index.lock'), '');
    assert.notEqual(git('write-tree').status, 0);
    assert.equal(stagedTree(repo), expected);
  } finally {
    rmSync(repo, { recursive: true, force: true });
  }
});

const BASE = { model: 'm', effort: 'high', report: '/r.md', prompt: 'p', home: '/Users/dev', root: '/repo' };

test('codexArgs uses the secret-denying read-only profile for a start and a resume', () => {
  const base = { model: 'm', effort: 'high', report: '/r.md', prompt: 'p', home: '/Users/dev' };
  for (const args of [codexArgs(base), codexArgs({ ...base, resume: 'id-1' })]) {
    assert.ok(!args.includes('-s'));
    assert.ok(args.includes('default_permissions="codex-review"'));
    const profile = args.find((arg) => arg.startsWith('permissions.codex-review='));
    assert.match(profile, /extends=":read-only"/);
    assert.match(profile, /"\/Users\/dev\/\.ssh"="deny"/);
    assert.match(profile, /"\/Users\/dev\/\.codex\/auth\.json"="deny"/);
    assert.match(profile, /":workspace_roots"=\{"\*\*\/\.secrets"="deny"/);
    // A -s flag or sandbox_mode silently replaces the profile, so neither must return.
    assert.ok(!args.some((arg) => arg.includes('sandbox_mode')));
    for (const path of ['.ssh', '.gnupg', '.aws', '.azure', '.config/gcloud', '.config/gh', '.kube', '.docker', '.netrc', '.npmrc', '.codex/auth.json']) {
      assert.ok(profile.includes(`"/Users/dev/${path}"="deny"`), path);
    }
    for (const pattern of ['**/.secrets', '**/.secrets/**', '**/.env', '**/.env.*']) {
      assert.ok(profile.includes(`"${pattern}"="deny"`), pattern);
    }
  }
  const resume = codexArgs({ ...base, resume: 'id-1' });
  assert.deepEqual(resume.slice(0, 3), ['exec', 'resume', 'id-1']);
  assert.ok(sandboxArgs('/home/a "b"').join(' ').includes('"/home/a \\"b\\"/.ssh"="deny"'));
});

test('the Codex runner passes the role settings and denies the Claude credentials', () => {
  const args = codexArgs({ ...BASE });
  assert.deepEqual(args.slice(-3), ['-o', '/r.md', 'p']);
  assert.ok(args.includes('m') && args.includes('model_reasoning_effort="high"'));
  for (const path of HOME_SECRETS) {
    assert.ok(args.join(' ').includes(`"/Users/dev/${path}"="deny"`), path);
  }
  for (const pattern of WORKSPACE_SECRETS) {
    assert.ok(args.join(' ').includes(`"${pattern}"="deny"`), pattern);
  }
  assert.equal(sessionID('no id\n'), undefined);
});

function option(args, name) {
  return args[args.indexOf(name) + 1];
}

test('the Claude runner limits tools and denies secrets for a start and a resume', () => {
  for (const args of [claude.args(BASE), claude.args({ ...BASE, resume: 'id-1' })]) {
    assert.deepEqual(args.slice(0, 2), ['-p', 'p']);
    assert.equal(option(args, '--model'), 'm');
    assert.equal(option(args, '--effort'), 'high');
    assert.equal(option(args, '--output-format'), 'json');
    assert.equal(option(args, '--tools'), 'Read,Grep,Glob,Bash');
    assert.equal(option(args, '--permission-mode'), 'dontAsk');
    assert.ok(args.includes('--restricted') && args.includes('--strict-mcp-config'));
    const settings = JSON.parse(option(args, '--settings'));
    assert.deepEqual(settings, claude.settings(BASE));
    for (const path of HOME_SECRETS) {
      assert.ok(settings.permissions.deny.includes(`Read(//Users/dev/${path})`), path);
      assert.ok(settings.permissions.deny.includes(`Read(//Users/dev/${path}/**)`), path);
      assert.ok(settings.sandbox.filesystem.denyRead.includes(`/Users/dev/${path}`), path);
    }
    for (const pattern of WORKSPACE_SECRETS) {
      assert.ok(settings.permissions.deny.includes(`Read(${pattern})`), pattern);
      assert.ok(settings.sandbox.filesystem.denyRead.includes(`/repo/${pattern}`), pattern);
    }
    assert.deepEqual(settings.sandbox.filesystem.denyWrite, ['/repo']);
    assert.equal(settings.sandbox.enabled, true);
    assert.equal(settings.sandbox.failIfUnavailable, true);
    assert.equal(settings.sandbox.allowUnsandboxedCommands, false);
  }
  assert.deepEqual(claude.args({ ...BASE, resume: 'id-1' }).slice(-2), ['--resume', 'id-1']);
  assert.ok(!claude.args(BASE).includes('--resume'));
  const worktree = JSON.parse(option(claude.args({ ...BASE, gitDirs: ['/main/.git/worktrees/wt', '/main/.git'] }), '--settings'));
  assert.deepEqual(worktree.sandbox.filesystem.denyWrite, ['/repo', '/main/.git/worktrees/wt', '/main/.git']);
  const quoted = claude.settings({ home: '/home/a "b"', root: '/my repo' });
  assert.ok(quoted.sandbox.filesystem.denyRead.includes('/home/a "b"/.ssh'));
  assert.deepEqual(quoted.sandbox.filesystem.denyWrite, ['/my repo']);
});

test('the Claude runner reads the report and session ID from its JSON result only', () => {
  const ok = JSON.stringify({ type: 'result', is_error: false, session_id: 's-1', result: '**Verdict:** APPROVE\n' });
  assert.equal(claude.report(`warning: something\n${ok}\n`), '**Verdict:** APPROVE\n');
  assert.equal(claude.sessionID(`warning\n${ok}\n`), 's-1');
  const failed = JSON.stringify({ type: 'result', is_error: true, session_id: 's-2', result: 'Verdict: APPROVE' });
  assert.equal(claude.report(failed), '');
  assert.equal(claude.sessionID(failed), 's-2');
  assert.equal(claude.report('Verdict: APPROVE\n'), '');
  assert.equal(claude.report(JSON.stringify({ type: 'other', result: 'Verdict: APPROVE' })), '');
  assert.equal(claude.sessionID('not json\n'), undefined);
});

// A runner fixture whose model names the behavior: `approve` writes an approving report,
// `fail` writes one and exits with an error, `print` prints the report for report(), and
// `silent` writes nothing.
const FAKE_RUNNER = `
export const command = process.execPath;
export const efforts = ['low', 'high'];
export function args({ model, report, prompt }) {
  return ['-e', \`
const fs = require('node:fs');
const [mode, report, prompt] = process.argv.slice(1);
fs.writeFileSync(report + '.prompt', prompt);
console.log('session id: fake-1');
if (mode === 'approve' || mode === 'fail') fs.writeFileSync(report, '**Verdict:** APPROVE\\\\n');
if (mode === 'print') console.log('REPORT **Verdict:** APPROVE');
process.exit(mode === 'fail' ? 1 : 0);
\`, model, report, prompt];
}
export function sessionID(log) {
  return log.match(/^session id: (\\S+)/m)?.[1];
}
`;
const PRINT_RUNNER = `${FAKE_RUNNER}
export function report(log) {
  return log.match(/^REPORT (.*)$/m)?.[1] ?? '';
}
`;

function reviewRepo(roles) {
  const { repo, git } = gitRepo('review-run-');
  mkdirSync(join(repo, '.agents/scripts/review-runners'), { recursive: true });
  mkdirSync(join(repo, '.agents'), { recursive: true });
  writeFileSync(join(repo, '.agents/scripts/review-runners/fake.mjs'), FAKE_RUNNER);
  writeFileSync(join(repo, '.agents/scripts/review-runners/printer.mjs'), PRINT_RUNNER);
  writeFileSync(join(repo, '.agents/review-roles.json'), JSON.stringify(roles));
  writeFileSync(join(repo, 'prompt.md'), 'Review this.\n');
  writeFileSync(join(repo, 'pr.md'), 'Description\n');
  git('add', '.');
  git('commit', '-q', '-m', 'init');
  writeFileSync(join(repo, 'a.txt'), 'change\n');
  git('add', 'a.txt');
  return { repo, git, tree: git('write-tree').stdout.trim() };
}

function review(repo, role, out, ...extra) {
  return spawnSync(process.execPath, [SCRIPT, role, '--out', join(repo, out), '--prompt-file', join(repo, 'prompt.md'), ...extra], {
    cwd: repo,
    encoding: 'utf8',
  });
}

const reviewsOf = (repo, role) => (existsSync(join(repo, '.git/reviews', role)) ? readdirSync(join(repo, '.git/reviews', role)) : []);

test('a review runs the runner of its role and stamps the tree and text only on approval', () => {
  const roles = {
    approver: { runner: 'fake', model: 'approve', effort: 'high' },
    failer: { runner: 'fake', model: 'fail', effort: 'high' },
    silent: { runner: 'fake', model: 'silent', effort: 'high' },
    printer: { runner: 'printer', model: 'print', effort: 'low' },
    missing: { runner: 'nope', model: 'approve', effort: 'high' },
    badEffort: { runner: 'fake', model: 'approve', effort: 'max' },
  };
  const { repo, tree } = reviewRepo(roles);
  try {
    const approved = review(repo, 'approver', 'out/a', '--stamp-file', join(repo, 'pr.md'));
    assert.equal(approved.status, 0, approved.stderr);
    assert.match(approved.stdout, /role: approver \(fake, approve, high\)/);
    assert.match(approved.stdout, /session id: fake-1/);
    assert.match(approved.stdout, /verdict: APPROVE/);
    const hash = createHash('sha256').update('Description\n').digest('hex');
    assert.deepEqual(reviewsOf(repo, 'approver').sort(), [tree, `text-${hash}`].sort());
    assert.match(readFileSync(join(repo, 'out/a.md.prompt'), 'utf8'), new RegExp(`git diff \\w+ ${tree}`));

    // An approving report does not count when the runner fails.
    const failed = review(repo, 'failer', 'out/f');
    assert.equal(failed.status, 1);
    assert.match(failed.stdout, /verdict: not approved/);
    assert.deepEqual(reviewsOf(repo, 'failer'), []);

    // A report from an earlier run with the same prefix does not count.
    writeFileSync(join(repo, 'out/s.md'), '**Verdict:** APPROVE\n');
    const silent = review(repo, 'silent', 'out/s');
    assert.match(silent.stdout, /verdict: not approved/);
    assert.deepEqual(reviewsOf(repo, 'silent'), []);

    const printed = review(repo, 'printer', 'out/p');
    assert.match(printed.stdout, /verdict: APPROVE/);
    assert.equal(readFileSync(join(repo, 'out/p.md'), 'utf8'), '**Verdict:** APPROVE');
    assert.deepEqual(reviewsOf(repo, 'printer'), [tree]);

    for (const role of ['missing', 'unknown-role', 'badEffort']) {
      const result = review(repo, role, 'out/x');
      assert.equal(result.status, 2, role);
      assert.equal(existsSync(join(repo, '.git/reviews', role)), false, role);
    }
    assert.match(review(repo, 'unknown-role', 'out/x').stderr, /no role unknown-role/);
    assert.match(review(repo, 'missing', 'out/x').stderr, /names the runner nope, but \.agents\/scripts\/review-runners\/ has only: fake, printer/);
    assert.match(review(repo, 'badEffort', 'out/x').stderr, /the fake runner does not accept the effort max/);
  } finally {
    rmSync(repo, { recursive: true, force: true });
  }
});
