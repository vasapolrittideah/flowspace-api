import assert from 'node:assert/strict';
import test from 'node:test';

import { codexArgs, isApproved, roleSettings, sessionID, stampPath } from './codex-review.mjs';
import { commitContentProblem, decide, gitCommands, pushTargets, shellCommands } from './git-guard.mjs';

const COMMON = '/repo/.git';
const TREE = 'tree-staged';

function fakeGit({ staged = 'cmd/main.go', headTree = 'tree-head', prs = {}, ghStatus = 0 } = {}) {
  const outputs = {
    'write-tree': TREE,
    'rev-parse --git-common-dir': COMMON,
    'rev-parse HEAD^{tree}': headTree,
    'rev-parse --show-toplevel': '/repo',
    'rev-parse --abbrev-ref HEAD': 'feat/current',
    'diff --cached --name-only': staged,
  };
  const calls = [];
  const run = (cmd, args, cwd) => {
    calls.push({ cmd, args, cwd });
    if (cmd === 'gh') {
      return { status: ghStatus, stdout: JSON.stringify(prs[args[3]] ?? []) };
    }
    const key = args.join(' ');
    return key in outputs ? { status: 0, stdout: `${outputs[key]}\n` } : { status: 1, stdout: '' };
  };
  run.calls = calls;
  return run;
}

function existing(...paths) {
  return (path) => paths.includes(path);
}

const STAMP = stampPath(COMMON, 'code-reviewer', TREE);
const decideWith = (command, run = fakeGit(), exists = existing()) => decide({ command, cwd: '/repo' }, { run, exists });

test('shellCommands respects quotes and shell operators', () => {
  assert.deepEqual(shellCommands(`printf '%s' 'a; git commit; b' && (git status)`), [
    ['printf', '%s', 'a; git commit; b'],
    ['git', 'status'],
  ]);
  assert.deepEqual(shellCommands('git -C "/my repo" commit --file m.txt'), [
    ['git', '-C', '/my repo', 'commit', '--file', 'm.txt'],
  ]);
});

test('gitCommands follows -C, wrappers, subshells, and line continuations', () => {
  assert.deepEqual(gitCommands('GIT_X=1 command git -C ../wt -C nested commit', '/repo'), [
    { dir: '/wt/nested', sub: 'commit', args: [] },
  ]);
  assert.deepEqual(gitCommands('cd sub', '/repo'), [{ dir: '/repo', sub: 'cd', args: ['sub'] }]);
  assert.deepEqual(gitCommands('git \\\ncommit -m fix', '/repo'), [{ dir: '/repo', sub: 'commit', args: ['-m', 'fix'] }]);
  assert.deepEqual(gitCommands('git push origin "a \\\nb"', '/repo'), [{ dir: '/repo', sub: 'push', args: ['origin', 'a b'] }]);
  assert.deepEqual(gitCommands('(git commit --file m.txt)', '/repo'), [{ dir: '/repo', sub: 'commit', args: ['--file', 'm.txt'] }]);
  assert.deepEqual(gitCommands(`echo 'git commit'`, '/repo'), []);
});

test('commit without a code-reviewer stamp is blocked', () => {
  const decision = decideWith('git commit --file m.txt');
  assert.equal(decision.block, true);
  assert.match(decision.reason, /code-reviewer/);
});

test('commit with a stamp for the staged tree is allowed', () => {
  assert.equal(decideWith('git commit --file m.txt', fakeGit(), existing(STAMP)).block, false);
});

test('a stamp for an older tree does not allow the commit', () => {
  const old = stampPath(COMMON, 'code-reviewer', 'tree-before-edit');
  assert.equal(decideWith('git commit --file m.txt', fakeGit(), existing(old)).block, true);
});

test('message-only amend needs no new review', () => {
  assert.equal(decideWith('git commit --amend --file m.txt', fakeGit({ headTree: TREE })).block, false);
});

test('commits that can include unstaged changes are blocked before any check', () => {
  const run = fakeGit({ headTree: TREE });
  for (const command of [
    'git add -A && git commit --file m.txt',
    'git commit -am fix',
    'git commit -a --file m.txt',
    'git commit --file m.txt -- cmd/main.go',
    'git commit --file m.txt cmd/main.go',
    'git commit --only --file m.txt',
    'git commit -am"fix bug"',
    'git \\\ncommit -a -m fix',
  ]) {
    assert.equal(decideWith(command, run, existing(STAMP)).block, true, command);
  }
  assert.equal(commitContentProblem(['-m', 'subject with -a', '--amend']), undefined);
  assert.equal(commitContentProblem(['-mfix']), undefined);
  assert.equal(commitContentProblem(['-s', '--file', 'm.txt']), undefined);
  assert.equal(commitContentProblem(['-sm', 'Fix issue']), undefined);
  assert.equal(commitContentProblem(['-sm', 'Fix issue', 'cmd/main.go']), 'it names paths');
});

test('a commit or a push that shares its line with cd is blocked', () => {
  const decision = decideWith('(cd ../other && pwd); git commit -m fix', fakeGit(), existing(STAMP));
  assert.equal(decision.block, true);
  assert.match(decision.reason, /git -C/);
  assert.equal(decideWith('cd ../other && git push origin feat/x').block, true);
  assert.equal(decideWith('cd ../other && git status').block, false);
});

test('here-document bodies are data, not commands', () => {
  const command = "cat > p.txt <<'EOF'\nRun git switch x && git push origin HEAD\nEOF\nnode run.mjs <<-END\n\tgit commit -am x\n\tEND\ngit status";
  assert.deepEqual(gitCommands(command, '/repo'), [{ dir: '/repo', sub: 'status', args: [] }]);
  assert.equal(decideWith(command).block, false);
});

test('a here-document operator inside quotes or a comment hides no command', () => {
  const merged = fakeGit({ prs: { 'feat/current': [{ number: 1, state: 'MERGED' }] } });
  assert.equal(decideWith("echo 'Example: <<EOF'\ngit commit -m fix").block, true);
  assert.equal(decideWith('echo "<<EOF"\ngit commit -m fix').block, true);
  assert.equal(decideWith('# see <<EOF\ngit push origin feat/current', merged).block, true);
  assert.equal(decideWith('echo $((1<<2))\ngit commit -m fix').block, true);
});

test('a quoted git command is not treated as a commit', () => {
  assert.equal(decideWith(`printf '%s' 'example; git commit; end'`).block, false);
});

test('a staged migration also needs migration-reviewer when the role exists', () => {
  const run = fakeGit({ staged: 'services/identity/db/migrations/00009_add_x.sql' });
  const role = '/repo/.codex/agents/migration-reviewer.toml';
  const blocked = decideWith('git commit', run, existing(STAMP, role));
  assert.equal(blocked.block, true);
  assert.match(blocked.reason, /migration-reviewer/);

  const migration = stampPath(COMMON, 'migration-reviewer', TREE);
  assert.equal(decideWith('git commit', run, existing(STAMP, role, migration)).block, false);
  assert.equal(decideWith('git commit', run, existing(STAMP)).block, false);
});

test('push to a branch whose only PR is merged is blocked', () => {
  const run = fakeGit({ prs: { 'feat/current': [{ number: 361, state: 'MERGED' }] } });
  const decision = decideWith('git push -u origin feat/current', run);
  assert.equal(decision.block, true);
  assert.match(decision.reason, /#361 MERGED/);
});

test('push checks every branch that it sends', () => {
  const run = fakeGit({
    prs: { open: [{ number: 1, state: 'OPEN' }], merged: [{ number: 2, state: 'MERGED' }] },
  });
  const decision = decideWith('git push origin open merged', run);
  assert.equal(decision.block, true);
  assert.match(decision.reason, /merged/);
});

test('push is allowed with an open PR, no PR, no answer from gh, or a delete', () => {
  const merged = { 'feat/current': [{ number: 1, state: 'MERGED' }] };
  assert.equal(decideWith('git push origin HEAD:feat/current', fakeGit({ prs: { 'feat/current': [{ number: 1, state: 'MERGED' }, { number: 2, state: 'OPEN' }] } })).block, false);
  assert.equal(decideWith('git push origin HEAD:feat/current', fakeGit()).block, false);
  assert.equal(decideWith('git push origin HEAD:feat/current', fakeGit({ prs: merged, ghStatus: 1 })).block, false);
  assert.equal(decideWith('git push --delete origin feat/current', fakeGit({ prs: merged })).block, false);
  assert.equal(decideWith('git push origin :feat/current', fakeGit({ prs: merged })).block, false);
});

test('push with --all, --mirror, or the matching refspec is blocked', () => {
  assert.equal(decideWith('git push --all origin').block, true);
  assert.equal(decideWith('git push --mirror origin').block, true);
  assert.equal(decideWith('git push origin :').block, true);
});

test('push that shares its line with another git command is blocked', () => {
  const run = fakeGit({ prs: { 'feat/current': [{ number: 1, state: 'OPEN' }] } });
  const decision = decideWith('git switch merged-branch && git push origin HEAD', run);
  assert.equal(decision.block, true);
  assert.match(decision.reason, /git push/);
});

test('push option values do not hide a branch or fake a delete', () => {
  const run = fakeGit({ prs: { merged: [{ number: 2, state: 'MERGED' }] } });
  assert.equal(decideWith('git push --repo origin merged', run).block, true);
  assert.equal(decideWith('git push -o --delete origin merged', run).block, true);
});

test('pushTargets reads every refspec and skips option values', () => {
  assert.deepEqual(pushTargets(['-u', 'origin', 'feat/x'], 'main').branches, ['feat/x']);
  assert.deepEqual(pushTargets(['--force-with-lease=feat/x:abc', 'origin', 'HEAD:refs/heads/feat/x'], 'main').branches, ['feat/x']);
  assert.deepEqual(pushTargets(['-o', 'notify=none', 'origin', 'HEAD'], 'feat/y').branches, ['feat/y']);
  assert.deepEqual(pushTargets(['origin', 'a', '+b:c', 'refs/tags/v1'], 'main').branches, ['a', 'c']);
  assert.deepEqual(pushTargets([], 'feat/y').branches, ['feat/y']);
});

test('codex-review helpers read role settings, verdicts, and session IDs', () => {
  assert.deepEqual(roleSettings('model = "gpt-6-astra"\nmodel_reasoning_effort = "medium"\n'), {
    model: 'gpt-6-astra',
    effort: 'medium',
  });
  assert.throws(() => roleSettings('model = "gpt-6-astra"\n'));
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

test('codexArgs keeps the sandbox read-only for a start and a resume', () => {
  const base = { model: 'm', effort: 'high', report: '/r.md', prompt: 'p' };
  assert.deepEqual(codexArgs(base).slice(0, 3), ['exec', '-s', 'read-only']);
  const resume = codexArgs({ ...base, resume: 'id-1' });
  assert.deepEqual(resume.slice(0, 3), ['exec', 'resume', 'id-1']);
  assert.ok(resume.includes('sandbox_mode="read-only"'));
});
