import assert from 'node:assert/strict';
import test from 'node:test';

import { stampPath, textStampPath } from './review.mjs';
import { commitContentProblem, decide, ghPr, gitCommands, prBody, shellCommands } from './git-guard.mjs';

const COMMON = '/repo/.git';
const TREE = 'tree-staged';

function fakeGit({ staged = 'cmd/main.go', headTree = 'tree-head' } = {}) {
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
const FILES = { '/repo/pr.md': 'Approved description\n' };
const read = (path) => {
  if (!(path in FILES)) {
    throw new Error(`missing ${path}`);
  }
  return FILES[path];
};
const decideWith = (command, run = fakeGit(), exists = existing()) => decide({ command, cwd: '/repo' }, { run, exists, read });

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
    'git commit -i --file m.txt',
    'git commit -p --file m.txt',
    'git commit --patch --file m.txt',
    'git commit --pathspec-from-file=paths.txt --file m.txt',
  ]) {
    assert.equal(decideWith(command, run, existing(STAMP)).block, true, command);
  }
  assert.deepEqual(run.calls, []);
  assert.equal(commitContentProblem(['-m', 'subject with -a', '--amend']), undefined);
  assert.equal(commitContentProblem(['-mfix']), undefined);
  assert.equal(commitContentProblem(['-s', '--file', 'm.txt']), undefined);
  assert.equal(commitContentProblem(['-sm', 'Fix issue']), undefined);
  assert.equal(commitContentProblem(['-sm', 'Fix issue', 'cmd/main.go']), 'it names paths');
});

test('a commit that shares its line with cd or another git command is blocked', () => {
  const decision = decideWith('(cd ../other && pwd); git commit -m fix', fakeGit(), existing(STAMP));
  assert.equal(decision.block, true);
  assert.match(decision.reason, /git -C/);
  assert.equal(decideWith('git push origin feat/x && git commit -m fix', fakeGit(), existing(STAMP)).block, true);
  assert.equal(decideWith('git cherry-pick abc && git commit -m fix', fakeGit(), existing(STAMP)).block, true);
  assert.equal(decideWith('cd ../other && git status').block, false);
});

test('push and other git commands that create commits are not checked', () => {
  for (const command of [
    'git push origin main',
    'git push --all origin',
    'git switch x && git push origin HEAD',
    'cd ../other && git push origin feat/x',
    'git cherry-pick abc123',
    'git revert abc123',
    'git merge feat/other',
    'git rebase origin/main',
    'git am patch.mbox',
    'git pull',
  ]) {
    const run = fakeGit();
    assert.deepEqual(decideWith(command, run), { block: false }, command);
    assert.deepEqual(run.calls, [], command);
  }
});

test('here-document bodies are data, not commands', () => {
  const command = "cat > p.txt <<'EOF'\nRun git switch x && git push origin HEAD\nEOF\nnode run.mjs <<-END\n\tgit commit -am x\n\tEND\ngit status";
  assert.deepEqual(gitCommands(command, '/repo'), [{ dir: '/repo', sub: 'status', args: [] }]);
  assert.equal(decideWith(command).block, false);
});

test('a here-document operator inside quotes or a comment hides no command', () => {
  assert.equal(decideWith("echo 'Example: <<EOF'\ngit commit -m fix").block, true);
  assert.equal(decideWith('echo "<<EOF"\ngit commit -m fix').block, true);
  assert.equal(decideWith('# see <<EOF\ngit commit -m fix').block, true);
  assert.equal(decideWith('echo $((1<<2))\ngit commit -m fix').block, true);
});

test('a quoted git command is not treated as a commit', () => {
  assert.equal(decideWith(`printf '%s' 'example; git commit; end'`).block, false);
});

test('a staged migration also needs migration-reviewer', () => {
  const run = fakeGit({ staged: 'services/identity/db/migrations/00009_add_x.sql' });
  const blocked = decideWith('git commit', run, existing(STAMP));
  assert.equal(blocked.block, true);
  assert.match(blocked.reason, /migration-reviewer/);
  assert.match(blocked.reason, /See \.agents\/rules\/review-roles\.md\./);

  const migration = stampPath(COMMON, 'migration-reviewer', TREE);
  assert.equal(decideWith('git commit', run, existing(STAMP, migration)).block, false);
  assert.equal(decideWith('git commit', run, existing(migration)).block, true);
  assert.equal(decideWith('git commit', run, existing(STAMP, stampPath(COMMON, 'migration-reviewer', 'tree-old'))).block, true);
});

test('gh pr merge is always blocked', () => {
  assert.equal(decideWith('gh pr merge 12 --squash').block, true);
  assert.equal(decideWith('gh pr merge --auto --squash').block, true);
});

test('gh pr create and edit need a convention-reviewer approval for the tree and the description', () => {
  const run = fakeGit({ headTree: TREE });
  // The writing-reviewer approvals are in place, so this test isolates the convention-reviewer ones.
  const writing = [stampPath(COMMON, 'writing-reviewer', TREE), textStampPath(COMMON, 'writing-reviewer', FILES['/repo/pr.md'])];
  const existing = (...paths) => (path) => [...writing, ...paths].includes(path);
  const treeStamp = stampPath(COMMON, 'convention-reviewer', TREE);
  const textStamp = textStampPath(COMMON, 'convention-reviewer', FILES['/repo/pr.md']);
  const create = 'gh pr create --base main --title "feat: x" --body-file pr.md';

  assert.equal(decideWith(create, run, existing()).block, true);
  assert.equal(decideWith(create, run, existing(treeStamp)).block, true);
  assert.equal(decideWith(create, run, existing(treeStamp, textStamp)).block, false);

  const otherText = textStampPath(COMMON, 'convention-reviewer', 'An older description\n');
  assert.equal(decideWith(create, run, existing(treeStamp, otherText)).block, true);

  assert.equal(decideWith('gh pr create --title x --body "inline"', run, existing(treeStamp, textStamp)).block, true);
  assert.equal(decideWith('gh pr create --title x --fill', run, existing(treeStamp, textStamp)).block, true);
  assert.equal(decideWith('gh pr create --title x', run, existing(treeStamp, textStamp)).block, true);
  assert.equal(decideWith('gh pr create --title x --body-file missing.md', run, existing(treeStamp)).block, true);

  assert.equal(decideWith('gh pr edit 12 --body-file pr.md', run, existing(treeStamp, textStamp)).block, false);
  assert.equal(decideWith('gh pr edit 12 --add-label type:docs', run, existing(treeStamp)).block, false);
  assert.equal(decideWith('gh pr edit 12 --add-label type:docs', run, existing()).block, true);
  assert.equal(decideWith('gh pr view 12', run, existing()).block, false);
  assert.equal(decideWith('cd sub && gh pr edit 12 --body-file pr.md', run, existing(treeStamp, textStamp)).block, true);
});

const PR_ROLES = ['convention-reviewer', 'writing-reviewer'];
const prTreeStamps = (tree = TREE) => PR_ROLES.map((role) => stampPath(COMMON, role, tree));
const prTextStamps = (text = FILES['/repo/pr.md']) => PR_ROLES.map((role) => textStampPath(COMMON, role, text));
const PR_STAMPS = [...prTreeStamps(), ...prTextStamps()];

test('gh pr create and body edits also need the writing-reviewer approvals', () => {
  const run = fakeGit({ headTree: TREE });
  for (const command of ['gh pr create --base main --title "feat: x" --body-file pr.md', 'gh pr edit 12 --body-file pr.md']) {
    // Only the complete set of the four stamps allows the command.
    for (let mask = 0; mask < 16; mask += 1) {
      const stamps = PR_STAMPS.filter((_, i) => mask & (1 << i));
      assert.equal(decideWith(command, run, existing(...stamps)).block, mask !== 15, `${command} with stamp set ${mask}`);
    }
    // Each role must approve this exact text, down to the last byte.
    for (const role of PR_ROLES) {
      const others = PR_STAMPS.filter((path) => path !== textStampPath(COMMON, role, FILES['/repo/pr.md']));
      const older = textStampPath(COMMON, role, 'Approved description');
      assert.equal(decideWith(command, run, existing(...others, older)).block, true, `${command} with an older ${role} text`);
    }
  }
});

test('PR approvals of both reviewers cover the tree of HEAD, not the staged tree', () => {
  const run = fakeGit({ headTree: 'tree-head' });
  const command = 'gh pr edit 12 --body-file pr.md';
  assert.equal(decideWith(command, run, existing(...prTreeStamps('tree-head'), ...prTextStamps())).block, false);
  assert.equal(decideWith(command, run, existing(...PR_STAMPS)).block, true);
  for (const role of PR_ROLES) {
    const stamps = [...prTreeStamps('tree-head'), ...prTextStamps()].map((path) =>
      path === stampPath(COMMON, role, 'tree-head') ? stampPath(COMMON, role, TREE) : path,
    );
    assert.equal(decideWith(command, run, existing(...stamps)).block, true, `${role} approved the staged tree`);
  }
});

test('a label-only PR edit needs both tree approvals and no text approval', () => {
  const run = fakeGit({ headTree: TREE });
  const command = 'gh pr edit 12 --add-label type:docs';
  assert.equal(decideWith(command, run, existing(...prTreeStamps())).block, false);
  assert.equal(decideWith(command, run, existing()).block, true);
  for (const role of PR_ROLES) {
    assert.equal(decideWith(command, run, existing(stampPath(COMMON, role, TREE), ...prTextStamps())).block, true, `only ${role}`);
  }
});

test('a missing PR approval names each missing reviewer and its review command', () => {
  const run = fakeGit({ headTree: TREE });
  const command = 'gh pr create --title x --body-file pr.md';
  const missing = (stamps) => decideWith(command, run, existing(...stamps)).reason.match(/from: ([^.]*)\./)[1];
  assert.equal(missing([]), 'convention-reviewer, writing-reviewer');
  assert.equal(missing(PR_STAMPS.filter((path) => !path.includes('writing-reviewer'))), 'writing-reviewer');
  assert.equal(missing(PR_STAMPS.filter((path) => !path.includes('convention-reviewer'))), 'convention-reviewer');
  const mixed = [stampPath(COMMON, 'writing-reviewer', TREE), textStampPath(COMMON, 'convention-reviewer', FILES['/repo/pr.md'])];
  assert.equal(missing(mixed), 'convention-reviewer, writing-reviewer');
  const reason = decideWith(command, run, existing()).reason;
  for (const role of PR_ROLES) {
    assert.match(reason, new RegExp(`node scripts/review\\.mjs ${role} \\.\\.\\. --stamp-file <description-file>`));
  }
});

test('PR body and command rules still block with every approval in place', () => {
  const run = fakeGit({ headTree: TREE });
  const approved = existing(...PR_STAMPS);
  for (const command of ['gh pr create --title x --body "inline"', 'gh pr edit 12 -bunreviewed', 'gh pr create --title x --fill', 'gh pr create --title x']) {
    const decision = decideWith(command, run, approved);
    assert.equal(decision.block, true, command);
    assert.match(decision.reason, /--body-file/, command);
  }
  assert.match(decideWith('gh pr edit 12 --body-file pr.md --body-file other.md', run, approved).reason, /more than one body file/);
  for (const command of ['gh pr create --title x --body-file missing.md', 'gh pr edit 12 --body-file missing.md']) {
    assert.match(decideWith(command, run, approved).reason, /Cannot read the PR description file missing\.md/, command);
  }
  assert.match(decideWith('cd sub && gh pr edit 12 --body-file pr.md', run, approved).reason, /Do not combine `cd`/);
  assert.match(decideWith('git status && gh pr edit 12 --body-file pr.md', run, approved).reason, /as its own command/);
  for (const command of ['gh pr merge 12 --squash', 'gh pr merge --auto --squash']) {
    assert.match(decideWith(command, run, approved).reason, /maintainer merges/, command);
  }
  for (const command of ['gh pr edit 12 --body-file pr.md', 'gh pr edit 12 --body-file=pr.md', 'gh pr edit 12 -F pr.md', 'gh pr edit 12 -Fpr.md']) {
    assert.equal(decideWith(command, run, approved).block, false, command);
  }
});

const BRANCH_DIFF = 'diff --name-only origin/main...HEAD';
// This function extends fakeGit with an answer for the branch diff that the hook reads before a
// PR command.
function planningGit({ paths = 'docs/specs/identity.md\n', status = 0, headTree = TREE } = {}) {
  const base = fakeGit({ headTree });
  return (cmd, args, cwd) => (args.join(' ') === BRANCH_DIFF ? { status, stdout: paths } : base(cmd, args, cwd));
}
const PLANNING_STAMP = (tree = TREE) => stampPath(COMMON, 'planning-reviewer', tree);
const ALL_PR = [...PR_STAMPS, PLANNING_STAMP()];
const CREATE = 'gh pr create --title x --body-file pr.md';
const BODY_EDIT = 'gh pr edit 12 --body-file pr.md';
const LABEL_EDIT = 'gh pr edit 12 --add-label type:docs';

test('a branch that changes a planning artifact needs a planning-reviewer approval for the PR', () => {
  for (const path of ['docs/specs/identity.md', 'tasks/identity-provider-login.md', 'docs/adr/0040-example.md']) {
    const run = planningGit({ paths: `cmd/main.go\n${path}\n` });
    for (const command of [CREATE, BODY_EDIT, LABEL_EDIT]) {
      const decision = decideWith(command, run, existing(...PR_STAMPS));
      assert.equal(decision.block, true, `${command} with ${path}`);
      assert.match(decision.reason, /from: planning-reviewer\./, `${command} with ${path}`);
      assert.match(decision.reason, /node scripts\/review\.mjs planning-reviewer \.\.\.`/);
      assert.equal(decideWith(command, run, existing(...ALL_PR)).block, false, `${command} with ${path}`);
    }
  }
});

test('the planning-reviewer approval covers the tree of HEAD and needs no text stamp', () => {
  const run = planningGit({ headTree: 'tree-head' });
  const others = [...prTreeStamps('tree-head'), ...prTextStamps()];
  assert.equal(decideWith(CREATE, run, existing(...others, PLANNING_STAMP('tree-head'))).block, false);
  assert.equal(decideWith(CREATE, run, existing(...others, PLANNING_STAMP(TREE))).block, true);
  const planningText = textStampPath(COMMON, 'planning-reviewer', FILES['/repo/pr.md']);
  assert.equal(decideWith(CREATE, run, existing(...others, planningText)).block, true);
});

test('on a planning branch, only the complete set of approvals allows the PR', () => {
  const run = planningGit();
  for (const command of [CREATE, BODY_EDIT]) {
    for (let mask = 0; mask < 32; mask += 1) {
      const stamps = ALL_PR.filter((_, i) => mask & (1 << i));
      assert.equal(decideWith(command, run, existing(...stamps)).block, mask !== 31, `${command} with stamp set ${mask}`);
    }
  }
  const trees = [...prTreeStamps(), PLANNING_STAMP()];
  for (let mask = 0; mask < 8; mask += 1) {
    const stamps = trees.filter((_, i) => mask & (1 << i));
    assert.equal(decideWith(LABEL_EDIT, run, existing(...stamps)).block, mask !== 7, `label edit with stamp set ${mask}`);
  }
});

test('the block reason lists each missing reviewer once', () => {
  const run = planningGit();
  const missing = (stamps) => decideWith(CREATE, run, existing(...stamps)).reason.match(/from: ([^.]*)\./)[1];
  assert.equal(missing([]), 'convention-reviewer, writing-reviewer, planning-reviewer');
  assert.equal(missing(PR_STAMPS), 'planning-reviewer');
  assert.equal(missing(ALL_PR.filter((path) => !path.includes('convention-reviewer'))), 'convention-reviewer');
  assert.equal(missing([...PR_STAMPS.filter((path) => !path.includes('writing-reviewer'))]), 'writing-reviewer, planning-reviewer');
});

test('the planning check reads the branch diff, not the staged diff', () => {
  const stagedOnly = (cmd, args, cwd) =>
    args.join(' ') === 'diff --cached --name-only' ? { status: 0, stdout: 'docs/specs/identity.md\n' } : planningGit({ paths: 'cmd/main.go\n' })(cmd, args, cwd);
  assert.equal(decideWith(CREATE, stagedOnly, existing(...PR_STAMPS)).block, false);
});

test('no planning approval is needed for other paths or when the branch diff fails', () => {
  const approved = existing(...PR_STAMPS);
  assert.match(decideWith(CREATE, planningGit(), approved).reason, /from: planning-reviewer\./);
  for (const paths of ['', 'cmd/main.go\n', 'docs/specs-old/x.md\n', 'tasks-old/x.md\n', 'other/docs/adr/x.md\n']) {
    assert.equal(decideWith(CREATE, planningGit({ paths }), approved).block, false, JSON.stringify(paths));
  }
  for (const paths of ['', 'docs/specs/identity.md\n']) {
    assert.equal(decideWith(CREATE, planningGit({ paths, status: 1 }), approved).block, false, `failed diff ${JSON.stringify(paths)}`);
  }
  assert.match(decideWith(CREATE, planningGit({ status: 1 }), existing()).reason, /from: convention-reviewer, writing-reviewer\./);
});

test('gh pr is found after the repository option in either position', () => {
  assert.deepEqual(ghPr(['--repo', 'owner/repo', 'pr', 'merge', '12', '--squash']), { action: 'merge', args: ['12', '--squash'] });
  assert.deepEqual(ghPr(['-R', 'owner/repo', 'pr', 'create', '--body-file', 'pr.md']), { action: 'create', args: ['--body-file', 'pr.md'] });
  assert.deepEqual(ghPr(['pr', 'merge', '-R', 'owner/repo', '12']), { action: 'merge', args: ['12'] });
  assert.equal(ghPr(['issue', 'view', '3']), undefined);
  assert.equal(decideWith('gh --repo owner/repo pr merge 12 --squash').block, true);
  assert.equal(decideWith('gh -R owner/repo pr create --title x --body-file pr.md').block, true);
});

test('every body option is read, and only one body file counts', () => {
  assert.deepEqual(prBody(['-Fpr.md']), { file: 'pr.md' });
  assert.match(prBody(['-bunreviewed']).problem, /-b/);
  assert.match(prBody(['--body-file', 'pr.md', '--body-file', 'other.md']).problem, /more than one/);
  const run = fakeGit({ headTree: TREE });
  // The writing-reviewer approvals are in place, so this test isolates the body options.
  const writing = [stampPath(COMMON, 'writing-reviewer', TREE), textStampPath(COMMON, 'writing-reviewer', FILES['/repo/pr.md'])];
  const existing = (...paths) => (path) => [...writing, ...paths].includes(path);
  const treeStamp = stampPath(COMMON, 'convention-reviewer', TREE);
  const textStamp = textStampPath(COMMON, 'convention-reviewer', FILES['/repo/pr.md']);
  assert.equal(decideWith('gh pr edit 12 -bunreviewed', run, existing(treeStamp, textStamp)).block, true);
  assert.equal(decideWith('gh pr edit 12 --body-file pr.md --body-file other.md', run, existing(treeStamp, textStamp)).block, true);
  assert.equal(decideWith('gh pr edit 12 -Fpr.md', run, existing(treeStamp, textStamp)).block, false);
});
