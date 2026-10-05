import assert from 'node:assert/strict';
import { spawnSync } from 'node:child_process';
import { mkdirSync, mkdtempSync, rmSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import test from 'node:test';
import { fileURLToPath } from 'node:url';

import { checkBody, checkBranch, checkLabels, checkMessage, checkProse, checkSubject, loadRules, run } from './check-pr-metadata.mjs';

const root = fileURLToPath(new URL('..', import.meta.url));
const script = fileURLToPath(new URL('check-pr-metadata.mjs', import.meta.url));
const rules = loadRules(root);
const claude = 'Co-authored-by: Claude Opus 5.5 <noreply@anthropic.com>';

const body = (sections = {}) =>
  [
    '## What changed',
    '',
    sections.what ?? 'The checker runs in CI.',
    '',
    '## Why',
    '',
    'Reviewers skip rules that code can check.',
    '',
    '## Related issues',
    '',
    sections.related ?? 'n/a',
    '',
    '## Risks or limitations',
    '',
    'n/a',
    '',
    '## Follow-up tasks',
    '',
    sections.followUp ?? 'n/a',
    '',
    '🤖 Generated with [Claude Code](https://claude.com/claude-code)',
    '',
  ].join('\n');

test('reads every type and scope from the commit message convention', () => {
  assert.equal(rules.types.size, 11);
  assert.equal(rules.scopes.size, 19);
  assert.deepEqual([...rules.sharedScopes].sort(), ['authn', 'config', 'logging', 'postgrespool', 'requestid', 'tracing']);
  assert.ok(rules.labels.has('area:shared'));
});

test('accepts the example subjects and branch names of the conventions', () => {
  for (const subject of [
    'feat(workspace): allow owners to invite workspace members',
    'fix(work): reject task updates based on an outdated version',
    'test(identity): prove provider login against its specification',
    'docs(observability): approve observability logs and traces spec',
    'docs(adr): explain the choice of squash merging',
    'fix(agents): preserve multiline PR descriptions',
    'build(codegen): configure Buf to generate ConnectRPC clients',
    'ci: add pull request title validation',
    'feat(workspace)!: require a role when inviting members',
    'revert(agents): clarify convention headings',
    'fix(notifications): prevent duplicate delivery when an event is retried',
  ]) {
    assert.deepEqual(checkSubject(subject, rules).findings, [], subject);
  }
  assert.deepEqual(checkBranch('feat/identity-github-login', 'feat'), []);
  assert.deepEqual(checkBranch('test/identity-provider-account-recovery', 'test'), []);
  assert.deepEqual(checkBranch('docs/clarify-hexagonal-rules', 'docs'), []);
  assert.deepEqual(checkBranch('fix/duplicate-notifications', 'fix'), []);
});

test('rejects malformed subjects', () => {
  const findings = (subject) => checkSubject(subject, rules).findings;
  assert.equal(findings('feature(identity): add login').length, 1);
  assert.equal(findings('feat(identity,workspace): add login').length, 1);
  assert.equal(findings('feat(): add login').length, 1);
  assert.equal(findings('feat(identity) add login').length, 1);
  assert.equal(findings('feat(identity): Add login').length, 1);
  assert.equal(findings('feat(identity): add login.').length, 1);
  assert.equal(findings('feat(identity): add login\nsecond line').length, 1);
  assert.deepEqual(findings(`docs: ${'a'.repeat(66)}`), []);
  assert.equal(findings(`docs: ${'a'.repeat(67)}`).length, 1);
});

test('rejects malformed branch names', () => {
  for (const branch of [
    'feature/identity-github-login',
    'claude/identity-github-login',
    'feat/Identity-login',
    'feat/identity_login',
    'feat/identity--login',
    'feat/-identity',
    'feat/identity/login',
    'feat',
  ]) {
    assert.equal(checkBranch(branch, 'feat').length, 1, branch);
  }
  assert.equal(checkBranch('fix/identity-login', 'feat').length, 1);
});

test('accepts the example commit and squash messages of the commit message convention', () => {
  const checkpoints = [
    `feat(identity): store trace context with outbox events\n\nMigration 00009 adds nullable traceparent and tracestate columns to\nidentity_outbox_events, and the outbox insert stores the context of\nthe current span. TestOutboxStoresTraceContext checks the stored\nvalues in a PostgreSQL container.\n\nRefs: #304\n\n${claude}\n`,
    'docs(agents): clarify convention headings\n\nCo-authored-by: Codex <noreply@openai.com>\n',
    'docs(agents): clarify convention headings',
  ];
  for (const message of checkpoints) {
    assert.deepEqual(checkMessage(message, rules, { squash: false }), [], message);
  }
  const squashes = [
    `feat(identity): validate Google callbacks and issue handoff codes\n\nCloses: #217\n\n${claude}\n`,
    `feat(identity): create provider-only accounts from Google logins\n\nMigration 00008 allows an empty password hash. Password login and\nrecovery skip such accounts, and rolling back the migration fails\nwhile they exist.\n\nCloses: #219\n\n${claude}\n`,
    `feat(workspace)!: require a role when inviting members\n\nInviteMember rejects a request without a role with InvalidArgument.\nCallers must send the role field, which was optional before.\n\n${claude}\n`,
    `revert(agents): clarify convention headings\n\nThis reverts commit 2f77dde4c1b9a6e3d5f8a0b7c2e4d6f8a1b3c5e7.\n\nThe new headings broke links from other convention files.\n\n${claude}\n`,
    `fix: close one Issue and refer to others\n\nCloses: #100\nRefs: #2\nRefs: #10\n\nCo-authored-by: Ana <ana@example.com>\n${claude}\n`,
  ];
  for (const message of squashes) {
    assert.deepEqual(checkMessage(message, rules, { squash: true }), [], message);
  }
});

test('exempts long lines with a URL or code, but not the prose next to them', () => {
  const url = `See https://example.com/${'a'.repeat(70)} for the reason.`;
  const code = `Run \`${'b'.repeat(70)}\` first.`;
  const prose = 'c'.repeat(73);
  assert.deepEqual(checkMessage(`fix: keep links\n\n${url}\n${code}\n`, rules, { squash: true }), []);
  assert.deepEqual(checkMessage(`fix: keep links\n\n${url}\n${prose}\n`, rules, { squash: true }), [
    'line 4 has 73 characters, more than 72',
  ]);
  assert.deepEqual(checkMessage(`fix: keep links\n\n${'d'.repeat(72)}\n`, rules, { squash: true }), []);
  assert.deepEqual(checkMessage(`fix: keep code\n\nRun:\n\n\`\`\`text\n${'e'.repeat(80)}\n\`\`\`\n\n${prose}\n`, rules, { squash: true }), [
    'line 9 has 73 characters, more than 72',
  ]);
});

test('rejects message structure, footer, and trailer errors', () => {
  const check = (message, squash = false) => checkMessage(message, rules, { squash });
  assert.deepEqual(check('fix: a\nbody'), ['no blank line after the subject']);
  assert.deepEqual(check('fix: a\n\nCloses: #217'), ['a checkpoint commit uses Refs, not Closes']);
  assert.deepEqual(check('fix: a\n\nRefs #217'), ['"Refs #217" is not a footer of the form Closes: #<n> or Refs: #<n>']);
  assert.deepEqual(check('fix: a\n\nFixes: #217', true), ['"Fixes: #217" is not a footer of the form Closes: #<n> or Refs: #<n>']);
  assert.deepEqual(check('fix: a\n\nRefs: #10\nRefs: #2'), ['the Issue footers are not Closes then Refs, each in ascending Issue number']);
  assert.deepEqual(check('fix: a\n\nRefs: #2\nCloses: #10', true), ['the Issue footers are not Closes then Refs, each in ascending Issue number']);
  assert.deepEqual(check('fix: a\n\nRefs: #2\n\nRefs: #10'), ['the Issue footers are not on consecutive lines']);
  assert.deepEqual(check('fix: a\n\nBody.\nRefs: #2'), ['put a blank line before the Issue footers']);
  assert.deepEqual(check('fix: a\n\nRefs: #2\n\nMore body.'), ['the Issue footers must come after the body']);
  assert.deepEqual(check(`fix: a\n\n${claude}\nRefs: #2`), ['put a blank line before the Issue footers', 'the Co-authored-by trailers must come at the end of the message']);
  assert.deepEqual(check('fix: a\n\nCo-Authored-By: Claude <noreply@anthropic.com>'), ['"Co-Authored-By" must be spelled Co-authored-by']);
  assert.deepEqual(check(`fix: a\n\n${claude}\nCo-authored-by: Ana <zz@example.com>`), [
    'the Co-authored-by trailers are not in alphabetical order',
  ]);
  // Prose that starts with an Issue keyword or holds a colon is body text.
  assert.deepEqual(check('fix: a\n\nFixes the race: the lock now covers the send.\nRefs: are parsed later.'), []);
});

test('checks labels against the title', () => {
  const labels = (list, title, migration = false) => checkLabels(list, checkSubject(title, rules), rules, { migration });
  assert.deepEqual(labels(['type:feat', 'area:identity'], 'feat(identity): add login'), []);
  assert.deepEqual(labels(['type:fix', 'area:shared'], 'fix(authn): reject expired tokens'), []);
  assert.deepEqual(labels(['type:refactor', 'area:shared'], 'refactor(shared): move clocks'), []);
  assert.deepEqual(labels(['type:ci'], 'ci: add checks'), []);
  assert.deepEqual(labels(['type:ci', 'area:infra', 'area:agents'], 'ci: add checks'), []);
  assert.deepEqual(labels(['type:feat', 'area:workspace', 'breaking'], 'feat(workspace)!: require roles'), []);
  assert.deepEqual(labels(['type:feat', 'area:identity', 'migration'], 'feat(identity): add login', true), []);

  assert.deepEqual(labels(['type:feat', 'area:identity', 'urgent'], 'feat(identity): add login'), ['label "urgent" is not in .github/labels.json']);
  assert.deepEqual(labels(['area:identity'], 'feat(identity): add login'), ['apply exactly one type label, type:feat']);
  assert.deepEqual(labels(['type:feat', 'type:fix', 'area:identity'], 'feat(identity): add login'), ['apply exactly one type label, type:feat']);
  assert.deepEqual(labels(['type:fix', 'area:identity'], 'feat(identity): add login'), ['apply exactly one type label, type:feat']);
  assert.deepEqual(labels(['type:feat'], 'feat(identity): add login'), ['apply only the area label area:identity']);
  assert.deepEqual(labels(['type:feat', 'area:identity', 'area:infra'], 'feat(identity): add login'), ['apply only the area label area:identity']);
  assert.deepEqual(labels(['type:fix', 'area:authn'], 'fix(authn): reject expired tokens'), [
    'label "area:authn" is not in .github/labels.json',
    'apply only the area label area:shared',
  ]);
  assert.deepEqual(labels(['type:feat', 'area:workspace'], 'feat(workspace)!: require roles'), ['apply breaking, because the title has !']);
  assert.deepEqual(labels(['type:feat', 'area:workspace', 'breaking'], 'feat(workspace): require roles'), ['remove breaking, because the title has no !']);
  assert.deepEqual(labels(['type:feat', 'area:identity'], 'feat(identity): add login', true), ['apply migration, because the diff changes a migration']);
  assert.deepEqual(labels(['type:feat', 'area:identity', 'migration'], 'feat(identity): add login'), ['remove migration, because the diff changes no migration']);
});

test('accepts complete PR descriptions', () => {
  assert.deepEqual(checkBody(body()), []);
  assert.deepEqual(checkBody(body().replace(/\n/g, '\r\n')), []);
  assert.deepEqual(
    checkBody(
      body({
        what: '- The checker runs in CI.\n- [The template](.github/pull_request_template.md) is unchanged.\n\n| Check | Result | Stack |\n| --- | --- | --- |\n| `task smoke:bruno` | Passed 44/44 requests | k3d |',
        related: 'Closes #12.\nCloses #100.\nRefs #3.',
        followUp: '- #40: Add the squash footer check.\n- #41: Check the follow-up order.\n- Rename the hexagonal convention file.',
      }),
    ),
    [],
  );
});

test('rejects malformed PR descriptions', () => {
  assert.deepEqual(checkBody(body().replace('## Why', '## Reason')), [
    'use the headings What changed, Why, Related issues, Risks or limitations, Follow-up tasks, in this order',
  ]);
  assert.deepEqual(checkBody(body({ what: '```markdown\n## Why\n```' }).replace('## Why\n\nReviewers', 'Reviewers')), [
    'use the headings What changed, Why, Related issues, Risks or limitations, Follow-up tasks, in this order',
  ]);
  assert.deepEqual(checkBody(body({ what: '<!-- State what changed. -->\nIt runs.' })), ['delete the HTML comments of the template']);
  assert.deepEqual(checkBody(body({ what: '' })), ['section What changed is empty; write n/a if there is nothing to report']);
  assert.deepEqual(checkBody(body({ related: 'Closes #12' })), ['Related issues: "Closes #12" is not "Closes #<n>." or "Refs #<n>."']);
  assert.deepEqual(checkBody(body({ related: 'Fixes #12.' })), ['Related issues: "Fixes #12." is not "Closes #<n>." or "Refs #<n>."']);
  assert.deepEqual(checkBody(body({ related: 'Closes #12, #13.' })), ['Related issues: "Closes #12, #13." is not "Closes #<n>." or "Refs #<n>."']);
  assert.deepEqual(checkBody(body({ related: 'n/a\nCloses #12.' })), ['Related issues: "n/a" is not "Closes #<n>." or "Refs #<n>."']);
  assert.deepEqual(checkBody(body({ related: 'Refs #3.\nCloses #12.' })), ['the Issue lines are not Closes then Refs, each in ascending Issue number']);
  assert.deepEqual(checkBody(body({ followUp: 'Rename the file.' })), ['Follow-up tasks: write n/a or one bullet for each piece of work']);
  assert.deepEqual(checkBody(body({ followUp: '- Closes #40: add the check.' })), [
    'Follow-up tasks: do not put Closes, Fixes, or Resolves before an Issue number',
  ]);
  assert.deepEqual(checkBody(body({ followUp: '- Rename the file.\n- #40: Add the check.' })), [
    'Follow-up tasks: put the bullets with an Issue first, in ascending Issue number',
  ]);
  assert.deepEqual(checkBody(body({ followUp: '- #41: Add one.\n- #40: Add two.' })), [
    'Follow-up tasks: put the bullets with an Issue first, in ascending Issue number',
  ]);
  assert.deepEqual(checkBody(body({ related: 'Refs #40.', followUp: '- #40: Add the check.' })), [
    'Follow-up tasks: do not repeat #40 from Related issues',
  ]);
  // Bullets that blank lines separate are still part of the section.
  assert.deepEqual(checkBody(body({ followUp: '- #41: Add one.\n\n- #40: Add two.\n\n- Closes #42: Add three.' })), [
    'Follow-up tasks: do not put Closes, Fixes, or Resolves before an Issue number',
    'Follow-up tasks: put the bullets with an Issue first, in ascending Issue number',
  ]);
  assert.deepEqual(checkBody(body({ followUp: '- Add one.\nRename the file.' })), ['Follow-up tasks: write n/a or one bullet for each piece of work']);
});

function repo(t) {
  const dir = mkdtempSync(join(tmpdir(), 'flowspace-pr-metadata-'));
  t.after(() => rmSync(dir, { force: true, recursive: true }));
  const git = (...args) => {
    const result = spawnSync('git', ['-c', 'user.name=Test', '-c', 'user.email=test@example.com', ...args], { cwd: dir, encoding: 'utf8' });
    assert.equal(result.status, 0, result.stderr);
    return result.stdout;
  };
  const commit = (message, file = 'file.txt') => {
    mkdirSync(join(dir, file, '..'), { recursive: true });
    writeFileSync(join(dir, file), `${message}\n`);
    git('add', '.');
    git('commit', '-q', '-m', message);
  };
  git('init', '-q', '-b', 'main');
  return { dir, git, commit, runGit: (args) => git(...args) };
}

test('checks the commits after the merge base, without merge commits', (t) => {
  const { git, commit, runGit } = repo(t);
  commit('Bad old commit on main');
  git('switch', '-q', '-c', 'fix/login');
  commit('fix(identity): reject expired codes', 'services/identity/db/migrations/00010.sql');
  git('switch', '-q', 'main');
  commit('chore: touch main', 'services/workspace/db/migrations/00004.sql');
  git('switch', '-q', 'fix/login');
  git('merge', '-q', '--no-edit', 'main');
  commit('fix(identity): Reject reused codes.');
  const options = { base: 'main', title: 'fix(identity): reject expired codes', labels: 'type:fix,area:identity,migration' };
  assert.deepEqual(run(options, rules, { git: runGit }), [
    'Commit ' + git('rev-parse', '--short', 'HEAD').trim() + ': the description does not start with a lowercase letter',
    'Commit ' + git('rev-parse', '--short', 'HEAD').trim() + ': the description ends with a period',
  ]);
  // A migration on main after the branch point is not part of the PR.
  const noMigration = run({ ...options, labels: 'type:fix,area:identity' }, rules, { git: runGit });
  assert.ok(noMigration.includes('Labels: apply migration, because the diff changes a migration'));
});

test('checks only the title and labels of a PR that Renovate opens', (t) => {
  const { git, commit, runGit } = repo(t);
  commit('chore: start');
  git('switch', '-q', '-c', 'renovate/go-modules');
  commit('Update module example.com/x to v2');
  const read = () => 'Renovate description';
  const options = { base: 'main', author: 'renovate[bot]', title: 'chore(deps): update example.com/x to v2', labels: 'type:chore,area:deps', 'body-file': 'x' };
  assert.deepEqual(run(options, rules, { git: runGit, read }), []);
  assert.deepEqual(run({ ...options, title: 'Update module example.com/x to v2', 'body-file': undefined }, rules, { git: runGit }), [
    'PR title: "Update module example.com/x to v2" does not have the form <type>(<scope>): <description>',
  ]);
  // A person can name a branch renovate/, so the branch alone does not skip the checks.
  const person = run({ ...options, author: 'someone' }, rules, { git: runGit, read });
  assert.equal(person[0], 'Branch: branch type "renovate" differs from the PR title type "chore"');
  assert.ok(person.some((finding) => finding.startsWith('Commit ')));
  assert.ok(person.some((finding) => finding.startsWith('PR description: ')));
});

test('checks the squash message against the title', () => {
  const read = () => 'fix(identity): reject expired codes\n\nCloses: #217\n';
  const runGit = (args) => (args[0] === 'log' || args[0] === 'diff' ? '' : 'fix/login\n');
  const options = { title: 'fix(identity): reject expired codes', 'squash-file': 'x' };
  assert.deepEqual(run(options, rules, { git: runGit, read }), []);
  assert.deepEqual(run({ ...options, title: 'fix(identity): reject old codes' }, rules, { git: runGit, read }), [
    'Squash message: the subject differs from the PR title',
  ]);
});

test('the CLI exits 1 with each finding and 0 when the metadata follows the rules', (t) => {
  const { dir, git, commit } = repo(t);
  commit('chore: start');
  git('switch', '-q', '-c', 'ci/check-metadata');
  commit('ci: check PR metadata');
  const cli = (...args) =>
    spawnSync('node', [script, '--base', 'main', ...args], { cwd: dir, encoding: 'utf8', env: { ...process.env, GITHUB_ACTIONS: 'false' } });
  const passing = cli('--title', 'ci: check PR metadata', '--labels', 'type:ci');
  assert.equal(passing.status, 0, passing.stdout + passing.stderr);
  const failing = cli('--title', 'ci: Check PR metadata.', '--labels', 'type:docs');
  assert.equal(failing.status, 1);
  assert.deepEqual(failing.stdout.trim().split('\n'), [
    'PR title: the description does not start with a lowercase letter',
    'PR title: the description ends with a period',
    'Labels: apply exactly one type label, type:ci',
  ]);
  assert.notEqual(cli('--unknown', 'x').status, 0);
  // In GitHub Actions, the runner must not read workflow commands in quoted PR metadata.
  const actions = spawnSync('node', [script, '--base', 'main', '--title', '##[warning]forged'], {
    cwd: dir,
    encoding: 'utf8',
    env: { ...process.env, GITHUB_ACTIONS: 'true' },
  });
  const output = actions.stdout.trim().split('\n');
  const token = output[0].match(/^::stop-commands::([0-9a-f-]{36})$/)?.[1];
  assert.ok(token, output[0]);
  assert.ok(output.slice(1, -1).some((line) => line.includes('##[warning]forged')));
  assert.equal(output.at(-1), `::${token}::`);
});

const SHOULD = '"should" is a modal that simple-english forbids. Use can, will, or must';

test('prose units keep paragraphs, list items, and table rows apart', () => {
  assert.deepEqual(checkProse('It runs in order\n\nto finish.\n- in order\n1. to finish'), []);
  // Markdown keeps a paragraph on one line, so two lines never join.
  assert.deepEqual(checkProse('It runs in order\nto finish.'), []);
  assert.deepEqual(checkProse('| It should pass | yes |'), [SHOULD]);
});

test('code, URLs, comments, and headings are not prose', () => {
  assert.deepEqual(checkProse('```text\nIt should; can\'t — has been robust.\n```\nIt runs.'), []);
  assert.deepEqual(checkProse('~~~~\n```\nIt should.\n```\n~~~~\nIt runs.'), []);
  assert.deepEqual(checkProse('```\nIt runs.\n```\nIt should run.'), [SHOULD]);
  assert.deepEqual(checkProse('Run `it should; never` and ``a `b` should``.'), []);
  assert.deepEqual(checkProse('Read [the guide](https://example.com/should;could) and <https://example.com/may>.'), []);
  assert.deepEqual(checkProse('Read [what you should know](docs/x.md).'), [SHOULD]);
  assert.deepEqual(checkProse('See https://example.com/a.b?c=should. It runs.'), []);
  assert.deepEqual(checkProse('<!-- It should\nnot count; at all. -->\n## It should not count\nIt runs.'), []);
  assert.deepEqual(checkProse('Run this:\n\n    echo should;\n\tlet x = 1; // it\'s fine\n\nIt runs.'), []);
  assert.deepEqual(checkProse('fix: a\n\n1. It runs a long step\n   that should wrap.', { commit: true }), [SHOULD]);
});

test('long sentences are not findings, because the sentence limits are targets', () => {
  const long = `${Array.from({ length: 60 }, () => 'word').join(' ')}.`;
  assert.deepEqual(checkProse(long), []);
  assert.deepEqual(checkProse(`fix: a\n\n${long}`, { commit: true }), []);
});

test('each forbidden word and mark is found as a whole word only', () => {
  const rule = (text) => checkProse(text).map((finding) => finding.split(' ')[0]);
  for (const modal of ['should', 'WOULD', 'May', 'might', 'Could']) {
    assert.deepEqual(rule(`It ${modal} run.`), [`"${modal}"`], modal);
  }
  for (const contraction of ["don't", "isn't", "they're", "we've", "it'll", "I'm", "you'd", "it's", "That's", "there's", "here's", "what's", "let's", "won’t", "He's", "she's", "who's", "where's", "how's"]) {
    assert.deepEqual(rule(`Yes ${contraction} fine.`), [`"${contraction}"`], contraction);
  }
  for (const phrase of ['has been', 'Have been', 'simply', 'seamlessly', 'robust', 'powerful', 'comprehensive', 'leverage', 'crucial', 'in order to', 'It is worth noting', 'In conclusion']) {
    assert.equal(checkProse(`It ${phrase} done.`).length, 1, phrase);
  }
  assert.deepEqual(checkProse('A shoulder, mayhem, robustness, and leveraged work. The team\'s, Jane\'s, users\' and its data can, will, and must run. It had been done. It has completed. A pre-commit hook – fine.'), []);
  assert.deepEqual(checkProse('One; two.'), ['a semicolon joins two sentences. Write two sentences']);
  assert.deepEqual(checkProse('One — two.'), ['an em dash joins two parts. Write two sentences or name the relation']);
});

test('commit prose joins wrapped lines and skips the subject, footers, trailers, and the revert line', () => {
  // A phrase across two wrapped lines of one paragraph counts, but not across two paragraphs.
  assert.deepEqual(checkProse('fix: a\n\nIt runs in order\n\nto finish.', { commit: true }), []);
  assert.deepEqual(checkProse('fix: a\n\nIt runs in order\nto finish.', { commit: true }), ['"in order to" carries no fact']);
  const metadata = [
    'fix: it should simply work',
    '',
    'This reverts commit 2f77dde4c1b9a6e3d5f8a0b7c2e4d6f8a1b3c5e7.',
    '',
    'Closes: #217',
    'Refs: #304',
    '',
    'Co-authored-by: May Robust <may@example.com>',
  ].join('\n');
  assert.deepEqual(checkProse(metadata, { commit: true }), []);
  assert.equal(checkProse('fix: a\n\nRefs: the old code should go.', { commit: true }).length, 1);
  assert.deepEqual(checkProse(metadata.replace(/\n/g, '\r\n'), { commit: true }), []);
  assert.deepEqual(checkProse('', { commit: true }), []);
  assert.deepEqual(checkProse('   \n\n'), []);
});

test('run reports prose findings from each source', () => {
  const runGit = (args) => {
    if (args[0] === 'log') return 'abc1234\0fix: a\n\nIt should run.\n\x1e';
    if (args[0] === 'diff') return '';
    return 'fix/login\n';
  };
  const read = (path) => (path === 'body' ? body({ what: 'It could run.' }) : 'fix: a\n\nIt might run.\n');
  const findings = run({ title: 'fix: a', 'body-file': 'body', 'squash-file': 'squash' }, rules, { git: runGit, read });
  assert.deepEqual(findings, [
    'Commit abc1234: "should" is a modal that simple-english forbids. Use can, will, or must',
    'PR description: "could" is a modal that simple-english forbids. Use can, will, or must',
    'Squash message: "might" is a modal that simple-english forbids. Use can, will, or must',
  ]);
  // The script checks only the squash message, because Renovate writes its commits and description.
  const renovate = run({ title: 'fix: a', author: 'renovate[bot]', branch: 'renovate/x', 'body-file': 'body', 'squash-file': 'squash' }, rules, { git: runGit, read });
  assert.deepEqual(renovate, ['Squash message: "might" is a modal that simple-english forbids. Use can, will, or must']);
});
