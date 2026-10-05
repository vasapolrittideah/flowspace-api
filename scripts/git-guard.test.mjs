import assert from 'node:assert/strict';
import { spawnSync } from 'node:child_process';
import { mkdtempSync, rmSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import test from 'node:test';

import { createHash } from 'node:crypto';

import { codexArgs, isApproved, promptWithSnapshot, roleSettings, sandboxArgs, sessionID, stagedTree, stampPath, textStampPath } from './codex-review.mjs';
import { commitContentProblem, commitMakerProblem, decide, ghPr, gitCommands, prBody, pushTargets, shellCommands } from './git-guard.mjs';

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

test('push to main is blocked in every form', () => {
  const onMain = (cmd, args, cwd) =>
    args.join(' ') === 'rev-parse --abbrev-ref HEAD' ? { status: 0, stdout: 'main\n' } : fakeGit()(cmd, args, cwd);
  for (const command of ['git push origin main', 'git push origin HEAD:main', 'git push origin HEAD:refs/heads/main', 'git push --delete origin main', 'git push origin :main']) {
    assert.equal(decideWith(command).block, true, command);
  }
  assert.equal(decideWith('git push origin HEAD', onMain).block, true);
  assert.equal(decideWith('git push', onMain).block, true);
});

test('commands that create commits without git commit are blocked', () => {
  for (const command of [
    'git cherry-pick abc123',
    'git revert abc123',
    'git rebase origin/main',
    'git rebase --continue',
    'git am patch.mbox',
    'git pull',
    'git pull --rebase origin main',
    'git merge feat/other',
    'git merge -m "main" feat/other',
    'git merge --continue',
  ]) {
    assert.equal(decideWith(command).block, true, command);
  }
  for (const command of [
    'git cherry-pick --no-commit abc123',
    'git cherry-pick -n abc123',
    'git revert --no-commit abc123',
    'git rebase --abort',
    'git cherry-pick --abort',
    'git pull --ff-only',
    'git merge origin/main',
    'git merge -m "Merge main" origin/main',
    'git merge --no-commit feat/other',
    'git merge --ff-only feat/other',
    'git merge --abort',
  ]) {
    assert.equal(decideWith(command).block, false, command);
  }
  assert.match(commitMakerProblem('cherry-pick', ['abc']), /--no-commit/);
});

test('gh pr merge is always blocked', () => {
  assert.equal(decideWith('gh pr merge 12 --squash').block, true);
  assert.equal(decideWith('gh pr merge --auto --squash').block, true);
});

test('gh pr create and edit need a convention-reviewer approval for the tree and the description', () => {
  const run = fakeGit({ headTree: TREE });
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

const WRITING_ROLE = '/repo/.codex/agents/writing-reviewer.toml';
const PR_ROLES = ['convention-reviewer', 'writing-reviewer'];
const prTreeStamps = (tree = TREE) => PR_ROLES.map((role) => stampPath(COMMON, role, tree));
const prTextStamps = (text = FILES['/repo/pr.md']) => PR_ROLES.map((role) => textStampPath(COMMON, role, text));
const PR_STAMPS = [...prTreeStamps(), ...prTextStamps()];

test('with its role file, gh pr create and body edits also need the writing-reviewer approvals', () => {
  const run = fakeGit({ headTree: TREE });
  for (const command of ['gh pr create --base main --title "feat: x" --body-file pr.md', 'gh pr edit 12 --body-file pr.md']) {
    // Only the complete set of the four stamps allows the command.
    for (let mask = 0; mask < 16; mask += 1) {
      const stamps = PR_STAMPS.filter((_, i) => mask & (1 << i));
      assert.equal(decideWith(command, run, existing(WRITING_ROLE, ...stamps)).block, mask !== 15, `${command} with stamp set ${mask}`);
    }
    // Each role must approve this exact text, down to the last byte.
    for (const role of PR_ROLES) {
      const others = PR_STAMPS.filter((path) => path !== textStampPath(COMMON, role, FILES['/repo/pr.md']));
      const older = textStampPath(COMMON, role, 'Approved description');
      assert.equal(decideWith(command, run, existing(WRITING_ROLE, ...others, older)).block, true, `${command} with an older ${role} text`);
    }
  }
});

test('PR approvals of both reviewers cover the tree of HEAD, not the staged tree', () => {
  const run = fakeGit({ headTree: 'tree-head' });
  const command = 'gh pr edit 12 --body-file pr.md';
  assert.equal(decideWith(command, run, existing(WRITING_ROLE, ...prTreeStamps('tree-head'), ...prTextStamps())).block, false);
  assert.equal(decideWith(command, run, existing(WRITING_ROLE, ...PR_STAMPS)).block, true);
  for (const role of PR_ROLES) {
    const stamps = [...prTreeStamps('tree-head'), ...prTextStamps()].map((path) =>
      path === stampPath(COMMON, role, 'tree-head') ? stampPath(COMMON, role, TREE) : path,
    );
    assert.equal(decideWith(command, run, existing(WRITING_ROLE, ...stamps)).block, true, `${role} approved the staged tree`);
  }
});

test('with its role file, a label-only PR edit needs both tree approvals and no text approval', () => {
  const run = fakeGit({ headTree: TREE });
  const command = 'gh pr edit 12 --add-label type:docs';
  assert.equal(decideWith(command, run, existing(WRITING_ROLE, ...prTreeStamps())).block, false);
  assert.equal(decideWith(command, run, existing(WRITING_ROLE)).block, true);
  for (const role of PR_ROLES) {
    assert.equal(decideWith(command, run, existing(WRITING_ROLE, stampPath(COMMON, role, TREE), ...prTextStamps())).block, true, `only ${role}`);
  }
});

test('a missing PR approval names each missing reviewer and its review command', () => {
  const run = fakeGit({ headTree: TREE });
  const command = 'gh pr create --title x --body-file pr.md';
  const missing = (stamps) => decideWith(command, run, existing(WRITING_ROLE, ...stamps)).reason.match(/from: ([^.]*)\./)[1];
  assert.equal(missing([]), 'convention-reviewer, writing-reviewer');
  assert.equal(missing(PR_STAMPS.filter((path) => !path.includes('writing-reviewer'))), 'writing-reviewer');
  assert.equal(missing(PR_STAMPS.filter((path) => !path.includes('convention-reviewer'))), 'convention-reviewer');
  const mixed = [stampPath(COMMON, 'writing-reviewer', TREE), textStampPath(COMMON, 'convention-reviewer', FILES['/repo/pr.md'])];
  assert.equal(missing(mixed), 'convention-reviewer, writing-reviewer');
  const reason = decideWith(command, run, existing(WRITING_ROLE)).reason;
  for (const role of PR_ROLES) {
    assert.match(reason, new RegExp(`node scripts/codex-review\\.mjs ${role} \\.\\.\\. --stamp-file <description-file>`));
  }
});

test('PR body and command rules still block with every approval in place', () => {
  const run = fakeGit({ headTree: TREE });
  const approved = existing(WRITING_ROLE, ...PR_STAMPS);
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

const PLANNING_ROLE = '/repo/.codex/agents/planning-reviewer.toml';
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
      const decision = decideWith(command, run, existing(WRITING_ROLE, PLANNING_ROLE, ...PR_STAMPS));
      assert.equal(decision.block, true, `${command} with ${path}`);
      assert.match(decision.reason, /from: planning-reviewer\./, `${command} with ${path}`);
      assert.match(decision.reason, /node scripts\/codex-review\.mjs planning-reviewer \.\.\.`/);
      assert.equal(decideWith(command, run, existing(WRITING_ROLE, PLANNING_ROLE, ...ALL_PR)).block, false, `${command} with ${path}`);
    }
  }
});

test('the planning-reviewer approval covers the tree of HEAD and needs no text stamp', () => {
  const run = planningGit({ headTree: 'tree-head' });
  const others = [...prTreeStamps('tree-head'), ...prTextStamps()];
  assert.equal(decideWith(CREATE, run, existing(WRITING_ROLE, PLANNING_ROLE, ...others, PLANNING_STAMP('tree-head'))).block, false);
  assert.equal(decideWith(CREATE, run, existing(WRITING_ROLE, PLANNING_ROLE, ...others, PLANNING_STAMP(TREE))).block, true);
  const planningText = textStampPath(COMMON, 'planning-reviewer', FILES['/repo/pr.md']);
  assert.equal(decideWith(CREATE, run, existing(WRITING_ROLE, PLANNING_ROLE, ...others, planningText)).block, true);
});

test('on a planning branch, only the complete set of approvals allows the PR', () => {
  const run = planningGit();
  for (const command of [CREATE, BODY_EDIT]) {
    for (let mask = 0; mask < 32; mask += 1) {
      const stamps = ALL_PR.filter((_, i) => mask & (1 << i));
      assert.equal(decideWith(command, run, existing(WRITING_ROLE, PLANNING_ROLE, ...stamps)).block, mask !== 31, `${command} with stamp set ${mask}`);
    }
  }
  const trees = [...prTreeStamps(), PLANNING_STAMP()];
  for (let mask = 0; mask < 8; mask += 1) {
    const stamps = trees.filter((_, i) => mask & (1 << i));
    assert.equal(decideWith(LABEL_EDIT, run, existing(WRITING_ROLE, PLANNING_ROLE, ...stamps)).block, mask !== 7, `label edit with stamp set ${mask}`);
  }
});

test('the block reason lists each missing reviewer once', () => {
  const run = planningGit();
  const missing = (stamps) => decideWith(CREATE, run, existing(WRITING_ROLE, PLANNING_ROLE, ...stamps)).reason.match(/from: ([^.]*)\./)[1];
  assert.equal(missing([]), 'convention-reviewer, writing-reviewer, planning-reviewer');
  assert.equal(missing(PR_STAMPS), 'planning-reviewer');
  assert.equal(missing(ALL_PR.filter((path) => !path.includes('convention-reviewer'))), 'convention-reviewer');
  assert.equal(missing([...PR_STAMPS.filter((path) => !path.includes('writing-reviewer'))]), 'writing-reviewer, planning-reviewer');
});

test('the planning check reads the branch diff, not the staged diff', () => {
  const stagedOnly = (cmd, args, cwd) =>
    args.join(' ') === 'diff --cached --name-only' ? { status: 0, stdout: 'docs/specs/identity.md\n' } : planningGit({ paths: 'cmd/main.go\n' })(cmd, args, cwd);
  assert.equal(decideWith(CREATE, stagedOnly, existing(WRITING_ROLE, PLANNING_ROLE, ...PR_STAMPS)).block, false);
});

test('no planning approval is needed without the role file, for other paths, or when the branch diff fails', () => {
  const approved = existing(WRITING_ROLE, PLANNING_ROLE, ...PR_STAMPS);
  assert.equal(decideWith(CREATE, planningGit(), existing(WRITING_ROLE, ...PR_STAMPS)).block, false);
  for (const paths of ['', 'cmd/main.go\n', 'docs/specs-old/x.md\n', 'tasks-old/x.md\n', 'other/docs/adr/x.md\n']) {
    assert.equal(decideWith(CREATE, planningGit({ paths }), approved).block, false, JSON.stringify(paths));
  }
  for (const paths of ['', 'docs/specs/identity.md\n']) {
    assert.equal(decideWith(CREATE, planningGit({ paths, status: 1 }), approved).block, false, `failed diff ${JSON.stringify(paths)}`);
  }
  assert.match(decideWith(CREATE, planningGit({ status: 1 }), existing(WRITING_ROLE, PLANNING_ROLE)).reason, /from: convention-reviewer, writing-reviewer\./);
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
  const treeStamp = stampPath(COMMON, 'convention-reviewer', TREE);
  const textStamp = textStampPath(COMMON, 'convention-reviewer', FILES['/repo/pr.md']);
  assert.equal(decideWith('gh pr edit 12 -bunreviewed', run, existing(treeStamp, textStamp)).block, true);
  assert.equal(decideWith('gh pr edit 12 --body-file pr.md --body-file other.md', run, existing(treeStamp, textStamp)).block, true);
  assert.equal(decideWith('gh pr edit 12 -Fpr.md', run, existing(treeStamp, textStamp)).block, false);
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
