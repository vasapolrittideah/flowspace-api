// Runs one review role and records an approval stamp for the staged tree.
// .agents/review-roles.json gives the runner, model, and effort of each role. A runner is a
// module in .agents/scripts/review-runners/ that turns the role configuration into one command.
// Usage:
//   node .agents/scripts/review.mjs <role> --out <prefix> --prompt-file <file> [--input <file>]
//   node .agents/scripts/review.mjs <role> --out <prefix> --prompt-file <file> --resume <session-id>
// Add --stamp-file <file> to also stamp the exact text of a file, such as a PR description.
import { spawnSync } from 'node:child_process';
import { createHash } from 'node:crypto';
import { closeSync, copyFileSync, existsSync, mkdirSync, openSync, readFileSync, readdirSync, rmSync, writeFileSync } from 'node:fs';
import { homedir, tmpdir } from 'node:os';
import { dirname, join, resolve } from 'node:path';
import { fileURLToPath, pathToFileURL } from 'node:url';

export const CONFIG_PATH = '.agents/review-roles.json';
export const RUNNERS_DIR = '.agents/scripts/review-runners';

// Credentials in the home directory and local secrets in the repository. A read-only reviewer
// can still read them, and the runner sends what it reads to the model. No review needs them.
// Each runner must deny all of them.
export const HOME_SECRETS = ['.ssh', '.gnupg', '.aws', '.azure', '.config/gcloud', '.config/gh', '.kube', '.docker',
  '.netrc', '.npmrc', '.codex/auth.json', '.claude/.credentials.json'];
export const WORKSPACE_SECRETS = ['**/.secrets', '**/.secrets/**', '**/.env', '**/.env.*'];

export function stampPath(gitCommonDir, role, tree) {
  return join(gitCommonDir, 'reviews', role, tree);
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

// Returns the runner, model, and effort of one role from the text of the role configuration.
export function roleSettings(config, role) {
  let roles;
  try {
    roles = JSON.parse(config);
  } catch (error) {
    throw new Error(`${CONFIG_PATH} is not valid JSON: ${error.message}`);
  }
  const settings = Object.hasOwn(roles ?? {}, role) ? roles[role] : undefined;
  if (!settings) {
    throw new Error(`${CONFIG_PATH} has no role ${role}`);
  }
  for (const key of ['runner', 'model', 'effort']) {
    if (typeof settings[key] !== 'string' || settings[key] === '') {
      throw new Error(`${CONFIG_PATH} needs a ${key} for ${role}`);
    }
  }
  const { runner, model, effort } = settings;
  return { runner, model, effort };
}

// Returns the module path of a runner. The name is one plain word, so it cannot leave the
// runners directory.
export function runnerPath(root, name) {
  if (!/^[a-z][a-z0-9-]*$/.test(name)) {
    throw new Error(`runner name ${JSON.stringify(name)} must use lowercase letters, digits, and hyphens`);
  }
  return join(root, RUNNERS_DIR, `${name}.mjs`);
}

// A runner exports the command to start, the efforts that its CLI accepts, args() to build its
// arguments, and sessionID() to read the session ID from its output. If it prints the report
// instead of writing the report file, it also exports report() to read the report from its output.
export async function loadRunner(path) {
  let runner;
  try {
    runner = await import(pathToFileURL(path).href);
  } catch (error) {
    throw new Error(`cannot load runner ${path}: ${error.message}`);
  }
  if (typeof runner.command !== 'string' || typeof runner.args !== 'function' || typeof runner.sessionID !== 'function') {
    throw new Error(`runner ${path} must export command, args(), and sessionID()`);
  }
  if (!Array.isArray(runner.efforts) || runner.efforts.length === 0 || !runner.efforts.every((value) => typeof value === 'string')) {
    throw new Error(`runner ${path} must export efforts as a list of effort names`);
  }
  if (runner.report !== undefined && typeof runner.report !== 'function') {
    throw new Error(`runner ${path} exports report, but it is not a function`);
  }
  return runner;
}

// Loads the runner of a role and makes sure that its CLI accepts the effort of the role. Some CLIs
// ignore an unknown effort and run with their default, so the review can run with a different effort.
export async function roleRunner(root, settings) {
  const path = runnerPath(root, settings.runner);
  if (!existsSync(path)) {
    const names = readdirSync(join(root, RUNNERS_DIR))
      .filter((file) => file.endsWith('.mjs'))
      .map((file) => file.slice(0, -'.mjs'.length))
      .sort();
    throw new Error(`${CONFIG_PATH} names the runner ${settings.runner}, but ${RUNNERS_DIR}/ has only: ${names.join(', ')}`);
  }
  const runner = await loadRunner(path);
  if (!runner.efforts.includes(settings.effort)) {
    throw new Error(`the ${settings.runner} runner does not accept the effort ${settings.effort}. Use one of: ${runner.efforts.join(', ')}`);
  }
  return runner;
}

function parseArgs(argv) {
  const [role, ...rest] = argv;
  const options = {};
  for (let i = 0; i < rest.length; i += 2) {
    options[rest[i].replace(/^--/, '')] = rest[i + 1];
  }
  if (!role || !options.out || !options['prompt-file']) {
    throw new Error('usage: review.mjs <role> --out <prefix> --prompt-file <file> [--input <file> | --resume <id>]');
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
  const copy = join(tmpdir(), `review-index-${process.pid}-${Date.now()}`);
  copyFileSync(resolve(cwd, git(['rev-parse', '--git-path', 'index'], { cwd })), copy);
  try {
    return git(['write-tree'], { cwd, env: { GIT_INDEX_FILE: copy } });
  } finally {
    rmSync(copy, { force: true });
  }
}

function readOrEmpty(path) {
  try {
    return readFileSync(path, 'utf8');
  } catch {
    return '';
  }
}

async function main() {
  const options = parseArgs(process.argv.slice(2));
  const root = git(['rev-parse', '--show-toplevel']);
  const settings = roleSettings(readFileSync(join(root, CONFIG_PATH), 'utf8'), options.role);
  const runner = await roleRunner(root, settings);
  // The stamp covers the tree that is staged when the review starts. A later edit changes the tree.
  const tree = stagedTree();
  const head = git(['rev-parse', 'HEAD']);
  const headTree = git(['rev-parse', 'HEAD^{tree}']);
  const report = resolve(`${options.out}.md`);
  const logPath = resolve(`${options.out}.log`);
  mkdirSync(dirname(report), { recursive: true });
  // A report from an earlier run with the same prefix must not approve this run.
  rmSync(report, { force: true });

  const input = openSync(options.input ?? '/dev/null', 'r');
  const log = openSync(logPath, 'w');
  const stampText = options['stamp-file'] ? readFileSync(options['stamp-file'], 'utf8') : undefined;
  const prompt = promptWithSnapshot(readFileSync(options['prompt-file'], 'utf8'), {
    tree,
    head,
    headTree,
    text: stampText,
  });
  const commonDir = resolve(root, git(['rev-parse', '--git-common-dir']));
  const gitDirs = [...new Set([git(['rev-parse', '--absolute-git-dir']), commonDir])];
  const args = runner.args({ ...settings, report, prompt, resume: options.resume, home: homedir(), root, gitDirs });
  const run = spawnSync(runner.command, args, { cwd: root, stdio: [input, log, log] });
  closeSync(input);
  closeSync(log);

  const logText = readOrEmpty(logPath);
  if (runner.report) {
    writeFileSync(report, runner.report(logText));
  }
  const approved = run.status === 0 && isApproved(readOrEmpty(report));
  if (approved) {
    const stamps = [stampPath(commonDir, options.role, tree)];
    if (stampText !== undefined) {
      stamps.push(textStampPath(commonDir, options.role, stampText));
    }
    for (const stamp of stamps) {
      mkdirSync(dirname(stamp), { recursive: true });
      writeFileSync(stamp, `${settings.runner} ${settings.model} ${settings.effort}\n${report}\n`);
    }
  }
  process.stdout.write(
    [
      `role: ${options.role} (${settings.runner}, ${settings.model}, ${settings.effort})`,
      `session id: ${runner.sessionID(logText) ?? 'unknown'}`,
      `verdict: ${approved ? 'APPROVE' : 'not approved'}`,
      `staged tree: ${tree}${approved ? ' (stamped)' : ''}`,
      ...(options['stamp-file'] ? [`stamp file: ${options['stamp-file']}${approved ? ' (stamped)' : ''}`] : []),
      `report: ${report}`,
      `log: ${logPath}`,
      ...(run.error ? [`error: ${run.error.message}`] : []),
      '',
    ].join('\n'),
  );
  process.exit(run.status === 0 ? 0 : 1);
}

if (process.argv[1] === fileURLToPath(import.meta.url)) {
  main().catch((error) => {
    process.stderr.write(`${error.message}\n`);
    process.exit(2);
  });
}
