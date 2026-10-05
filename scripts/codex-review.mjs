// Runs one Codex review role and records an approval stamp for the staged tree.
// Usage:
//   node scripts/codex-review.mjs <role> --out <prefix> --prompt-file <file> [--input <file>]
//   node scripts/codex-review.mjs <role> --out <prefix> --prompt-file <file> --resume <session-id>
// Add --stamp-file <file> to also stamp the exact text of a file, such as a PR description.
import { spawnSync } from 'node:child_process';
import { createHash } from 'node:crypto';
import { closeSync, copyFileSync, mkdirSync, openSync, readFileSync, rmSync, writeFileSync } from 'node:fs';
import { homedir, tmpdir } from 'node:os';
import { dirname, join, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';

export function stampPath(gitCommonDir, role, tree) {
  return join(gitCommonDir, 'codex-reviews', role, tree);
}

export function textStampPath(gitCommonDir, role, text) {
  return stampPath(gitCommonDir, role, `text-${createHash('sha256').update(text).digest('hex')}`);
}

// Adds the exact change under review to the prompt. Tree objects never change, and the
// text snapshot is read once, so the stamps cover exactly what the reviewer saw.
export function promptWithSnapshot(prompt, { tree, head, headTree, text }) {
  // After a commit, the staged tree equals the tree of HEAD, so the staged diff is empty.
  // The reviewer must use the diff scope that the prompt gives, such as a branch diff.
  const scope =
    tree === headTree
      ? `Nothing is staged beyond HEAD, whose tree is ${tree}. Review the diff scope that the task above ` +
        'gives. An APPROVE verdict approves that tree for a pull request.'
      : `The staged tree under review is ${tree}. Before you give a verdict, read exactly this change with ` +
        `\`git diff ${head} ${tree}\`. An APPROVE verdict approves that tree for commit.`;
  const lines = [prompt.trimEnd(), '', scope];
  if (text !== undefined) {
    const hash = createHash('sha256').update(text).digest('hex');
    lines.push(
      '',
      `An APPROVE verdict also approves this exact PR description (sha256 ${hash}). Review it as written:`,
      '',
      '<pr-description>',
      text,
      '</pr-description>',
    );
  }
  return `${lines.join('\n')}\n`;
}

export function isApproved(report) {
  // Only a line that starts with the verdict field counts, so a quoted or mentioned earlier
  // verdict cannot approve a rejected tree. The template line `APPROVE | REQUEST CHANGES` and
  // more than one verdict field do not count as an approval.
  const verdicts = report
    .split('\n')
    .map((line) => line.match(/^[*\s]*Verdict:[*\s]*(.*?)[*\s]*$/)?.[1])
    .filter((value) => value !== undefined);
  return verdicts.length === 1 && verdicts[0] === 'APPROVE';
}

export function roleSettings(toml) {
  const model = toml.match(/^model\s*=\s*"([^"]+)"/m)?.[1];
  const effort = toml.match(/^model_reasoning_effort\s*=\s*"([^"]+)"/m)?.[1];
  if (!model || !effort) {
    throw new Error('role file needs model and model_reasoning_effort');
  }
  return { model, effort };
}

export function sessionID(log) {
  return log.match(/^session id:\s*(\S+)/m)?.[1];
}

// Credentials in the home directory and local secrets in the repository. A read-only sandbox
// can still read them, and Codex sends what it reads to the model. No review needs them.
const HOME_SECRETS = ['.ssh', '.gnupg', '.aws', '.azure', '.config/gcloud', '.config/gh', '.kube', '.docker',
  '.netrc', '.npmrc', '.codex/auth.json'];
const WORKSPACE_SECRETS = ['**/.secrets', '**/.secrets/**', '**/.env', '**/.env.*'];

// Returns the Codex options for a sandbox that reads like `:read-only` but denies the secrets.
// A denied directory also denies everything under it.
export function sandboxArgs(home) {
  const homeEntries = HOME_SECRETS.map((path) => `${JSON.stringify(join(home, path))}="deny"`);
  const workspaceEntries = WORKSPACE_SECRETS.map((pattern) => `${JSON.stringify(pattern)}="deny"`);
  const filesystem = [...homeEntries, `":workspace_roots"={${workspaceEntries.join(', ')}}`].join(', ');
  return [
    '-c',
    'default_permissions="codex-review"',
    '-c',
    `permissions.codex-review={extends=":read-only", filesystem={${filesystem}}}`,
  ];
}

export function codexArgs({ model, effort, report, prompt, resume, home }) {
  // `codex exec resume` keeps none of the settings of the session, so both forms pass them all.
  const settings = ['-m', model, '-c', `model_reasoning_effort="${effort}"`, ...sandboxArgs(home)];
  if (resume) {
    return ['exec', 'resume', resume, ...settings, '-o', report, prompt];
  }
  return ['exec', ...settings, '-o', report, prompt];
}

function parseArgs(argv) {
  const [role, ...rest] = argv;
  const options = {};
  for (let i = 0; i < rest.length; i += 2) {
    options[rest[i].replace(/^--/, '')] = rest[i + 1];
  }
  if (!role || !options.out || !options['prompt-file']) {
    throw new Error('usage: codex-review.mjs <role> --out <prefix> --prompt-file <file> [--input <file> | --resume <id>]');
  }
  if (options.input && options.resume) {
    throw new Error('--input does not work with --resume, because resume ignores standard input');
  }
  return { role, ...options };
}

function git(args, { cwd, env } = {}) {
  const result = spawnSync('git', args, { cwd, encoding: 'utf8', env: { ...process.env, ...env } });
  if (result.status !== 0) {
    throw new Error(`git ${args.join(' ')} failed: ${result.stderr}`);
  }
  return result.stdout.trim();
}

// Returns the staged tree. It reads a copy of the index, because `git write-tree` locks the
// index, and two reviews that start at the same time would fail on index.lock.
export function stagedTree(cwd = process.cwd()) {
  const copy = join(tmpdir(), `codex-review-index-${process.pid}-${Date.now()}`);
  copyFileSync(resolve(cwd, git(['rev-parse', '--git-path', 'index'], { cwd })), copy);
  try {
    return git(['write-tree'], { cwd, env: { GIT_INDEX_FILE: copy } });
  } finally {
    rmSync(copy, { force: true });
  }
}

function main() {
  const options = parseArgs(process.argv.slice(2));
  const root = git(['rev-parse', '--show-toplevel']);
  const settings = roleSettings(readFileSync(join(root, '.codex/agents', `${options.role}.toml`), 'utf8'));
  // The stamp covers the tree that is staged when the review starts. A later edit changes the tree.
  const tree = stagedTree();
  const head = git(['rev-parse', 'HEAD']);
  const headTree = git(['rev-parse', 'HEAD^{tree}']);
  const report = resolve(`${options.out}.md`);
  const logPath = resolve(`${options.out}.log`);
  mkdirSync(dirname(report), { recursive: true });

  const input = openSync(options.input ?? '/dev/null', 'r');
  const log = openSync(logPath, 'w');
  const stampText = options['stamp-file'] ? readFileSync(options['stamp-file'], 'utf8') : undefined;
  const prompt = promptWithSnapshot(readFileSync(options['prompt-file'], 'utf8'), {
    tree,
    head,
    headTree,
    text: stampText,
  });
  const run = spawnSync('codex', codexArgs({ ...settings, report, prompt, resume: options.resume, home: homedir() }), {
    cwd: root,
    stdio: [input, log, log],
  });
  closeSync(input);
  closeSync(log);

  const reportText = (() => {
    try {
      return readFileSync(report, 'utf8');
    } catch {
      return '';
    }
  })();
  const approved = run.status === 0 && isApproved(reportText);
  if (approved) {
    const commonDir = resolve(root, git(['rev-parse', '--git-common-dir']));
    const stamps = [stampPath(commonDir, options.role, tree)];
    if (stampText !== undefined) {
      stamps.push(textStampPath(commonDir, options.role, stampText));
    }
    for (const stamp of stamps) {
      mkdirSync(dirname(stamp), { recursive: true });
      writeFileSync(stamp, `${report}\n`);
    }
  }
  process.stdout.write(
    [
      `role: ${options.role} (${settings.model}, ${settings.effort})`,
      `session id: ${sessionID(readFileSync(logPath, 'utf8')) ?? 'unknown'}`,
      `verdict: ${approved ? 'APPROVE' : 'not approved'}`,
      `staged tree: ${tree}${approved ? ' (stamped)' : ''}`,
      ...(options['stamp-file'] ? [`stamp file: ${options['stamp-file']}${approved ? ' (stamped)' : ''}`] : []),
      `report: ${report}`,
      `log: ${logPath}`,
      '',
    ].join('\n'),
  );
  process.exit(run.status === 0 ? 0 : 1);
}

if (process.argv[1] === fileURLToPath(import.meta.url)) {
  try {
    main();
  } catch (error) {
    process.stderr.write(`${error.message}\n`);
    process.exit(2);
  }
}
