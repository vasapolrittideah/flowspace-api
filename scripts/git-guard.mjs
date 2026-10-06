// Claude Code runs this PreToolUse hook before each Bash command. It enforces the rules that
// the GitHub ruleset on main cannot enforce. The hook blocks these commands:
// - `git commit` without a reviewer approval for the staged tree.
// - `gh pr merge`.
// - `gh pr create` and `gh pr edit` without the approvals of convention-reviewer and
//   writing-reviewer, and of planning-reviewer when the branch changes a planning artifact.
// It guards against forgotten steps, not against a deliberate bypass.
import { spawnSync } from 'node:child_process';
import { existsSync, readFileSync } from 'node:fs';
import { resolve } from 'node:path';
import { fileURLToPath } from 'node:url';

import { stampPath, textStampPath } from './review.mjs';

const MIGRATION_PATH = /^services\/[^/]+\/db\/migrations\//;
const PLANNING_PATH = /^(docs\/specs\/|tasks\/|docs\/adr\/)/;
const WRAPPERS = new Set(['command', 'exec', 'env', 'time', 'nohup', 'builtin']);
const COMMIT_VALUE_OPTIONS = new Set(['-m', '-F', '-C', '-c', '-t', '--message', '--file', '--reuse-message',
  '--reedit-message', '--template', '--author', '--date', '--fixup', '--squash', '--trailer', '--cleanup']);
const COMMIT_CONTENT_OPTIONS = new Set(['-a', '--all', '-i', '--include', '-o', '--only', '-p', '--patch',
  '--interactive', '--pathspec-from-file']);

// Splits a shell line into simple commands. Each command is a list of words without quotes.
// Operators outside quotes end a command: ; & | ( ) { } and newlines. Comments and the bodies
// of here-documents are data, so they produce no commands.
export function shellCommands(line) {
  const commands = [];
  const heredocs = [];
  let words = [];
  let word = '';
  let inWord = false;
  let quote = '';
  const endWord = () => {
    if (inWord) {
      words.push(word);
    }
    word = '';
    inWord = false;
  };
  const endCommand = () => {
    endWord();
    if (words.length > 0) {
      commands.push(words);
    }
    words = [];
  };
  // Returns the index after the closing line of each pending here-document.
  const skipHeredocs = (from) => {
    let p = from;
    for (const { delimiter, dash } of heredocs) {
      while (p < line.length) {
        const newline = line.indexOf('\n', p);
        const end = newline === -1 ? line.length : newline;
        const text = line.slice(p, end);
        p = end + 1;
        if ((dash ? text.replace(/^\t+/, '') : text) === delimiter) {
          break;
        }
      }
    }
    heredocs.length = 0;
    return p;
  };
  for (let i = 0; i < line.length; i += 1) {
    const ch = line[i];
    if (quote) {
      if (ch === quote) {
        quote = '';
      } else if (ch === '\\' && quote === '"' && line[i + 1] === '\n') {
        i += 1;
      } else if (ch === '\\' && quote === '"' && i + 1 < line.length) {
        word += line[++i];
      } else {
        word += ch;
      }
    } else if (ch === "'" || ch === '"') {
      quote = ch;
      inWord = true;
    } else if (ch === '\\' && line[i + 1] === '\n') {
      i += 1;
    } else if (ch === '\\' && i + 1 < line.length) {
      word += line[++i];
      inWord = true;
    } else if (ch === '#' && !inWord) {
      while (i + 1 < line.length && line[i + 1] !== '\n') {
        i += 1;
      }
    } else if (line.startsWith('<<', i) && line[i + 2] !== '<' && line[i - 1] !== '<') {
      const match = line.slice(i + 2).match(/^(-?)[ \t]*(?:'([^']*)'|"([^"]*)"|([A-Za-z_][A-Za-z0-9_]*))/);
      endWord();
      if (match) {
        heredocs.push({ dash: match[1] === '-', delimiter: match[2] ?? match[3] ?? match[4] });
        i += 1 + match[0].length;
      } else {
        i += 1;
      }
    } else if (ch === '\n') {
      endCommand();
      if (heredocs.length > 0) {
        i = skipHeredocs(i + 1) - 1;
      }
    } else if (/\s/.test(ch)) {
      endWord();
    } else if (';&|(){}'.includes(ch)) {
      endCommand();
    } else {
      word += ch;
      inWord = true;
    }
  }
  endCommand();
  return commands;
}

// Returns each git command in a shell line with its directory, subcommand, and arguments.
// It follows `-C`. It does not follow `cd`, because a subshell limits the scope of `cd`,
// so `decide` rejects a commit that shares its line with `cd`.
export function gitCommands(line, cwd) {
  const found = [];
  for (const command of shellCommands(line)) {
    const words = [...command];
    while (words.length > 0 && (/^[A-Za-z_][A-Za-z0-9_]*=/.test(words[0]) || WRAPPERS.has(words[0]))) {
      words.shift();
    }
    if (words[0] === 'cd') {
      found.push({ dir: cwd, sub: 'cd', args: words.slice(1) });
      continue;
    }
    if (words[0] === 'gh') {
      found.push({ dir: cwd, sub: 'gh', args: words.slice(1) });
      continue;
    }
    if (words[0] !== 'git') {
      continue;
    }
    let gitDir = cwd;
    let i = 1;
    while (i < words.length && words[i].startsWith('-')) {
      if (words[i] === '-C') {
        gitDir = resolve(gitDir, words[i + 1] ?? '.');
        i += 2;
      } else if (words[i] === '-c') {
        i += 2;
      } else {
        i += 1;
      }
    }
    if (words[i]) {
      found.push({ dir: gitDir, sub: words[i], args: words.slice(i + 1) });
    }
  }
  return found;
}

// Returns why a commit can contain a tree other than the staged tree, or undefined.
export function commitContentProblem(args) {
  for (let i = 0; i < args.length; i += 1) {
    const arg = args[i];
    if (arg === '--') {
      return args[i + 1] ? 'it names paths' : undefined;
    }
    if (COMMIT_CONTENT_OPTIONS.has(arg) || arg.startsWith('--pathspec-from-file=')) {
      return `it uses ${arg}`;
    }
    if (COMMIT_VALUE_OPTIONS.has(arg)) {
      i += 1;
    } else if (/^-[a-zA-Z]./.test(arg)) {
      // A cluster such as -am: the flags before a value option are flags, the rest is its value.
      const flags = arg.slice(1);
      for (let j = 0; j < flags.length; j += 1) {
        if (COMMIT_CONTENT_OPTIONS.has(`-${flags[j]}`)) {
          return `it uses -${flags[j]}`;
        }
        if (COMMIT_VALUE_OPTIONS.has(`-${flags[j]}`)) {
          // A value option at the end of the cluster takes the next argument as its value.
          if (j === flags.length - 1) {
            i += 1;
          }
          break;
        }
      }
    } else if (!arg.startsWith('-')) {
      return 'it names paths';
    }
  }
  return undefined;
}

// Returns the action and the arguments of a `gh pr` command, or undefined. It skips the
// repository option, which gh accepts before and after `pr`.
export function ghPr(args) {
  const rest = [];
  let action;
  let sawPr = false;
  for (let i = 0; i < args.length; i += 1) {
    const arg = args[i];
    if (arg === '-R' || arg === '--repo') {
      i += 1;
    } else if (arg.startsWith('--repo=') || /^-R./.test(arg)) {
      continue;
    } else if (!sawPr) {
      if (arg === 'pr') {
        sawPr = true;
      } else if (!arg.startsWith('-')) {
        return undefined;
      }
    } else if (action === undefined && !arg.startsWith('-')) {
      action = arg;
    } else {
      rest.push(arg);
    }
  }
  return sawPr && action ? { action, args: rest } : undefined;
}

// Returns the body file of `gh pr create` or `gh pr edit`, or a problem when the body is not
// in exactly one file.
export function prBody(args) {
  const files = [];
  for (let i = 0; i < args.length; i += 1) {
    const arg = args[i];
    if (arg === '--body-file' || arg === '-F') {
      files.push(args[i + 1]);
      i += 1;
    } else if (arg.startsWith('--body-file=')) {
      files.push(arg.slice('--body-file='.length));
    } else if (/^-F./.test(arg)) {
      files.push(arg.slice(2));
    } else if (
      ['--body', '-b', '--fill', '--fill-first', '--fill-verbose', '-f'].includes(arg) ||
      arg.startsWith('--body=') ||
      /^-b./.test(arg)
    ) {
      return { problem: `it sets the body with ${arg.startsWith('--') ? arg.split('=')[0] : arg.slice(0, 2)}` };
    }
  }
  if (files.length > 1) {
    return { problem: 'it passes more than one body file' };
  }
  return { file: files[0] };
}

// Decides whether to block the command. `run(cmd, args, cwd)` returns { status, stdout }.
export function decide({ command, cwd }, { run, exists, read }) {
  const gits = gitCommands(command, cwd);
  const prOf = (git) => (git.sub === 'gh' ? ghPr(git.args) : undefined);
  const isPr = (git) => ['create', 'edit', 'merge'].includes(prOf(git)?.action);
  const guarded = gits.filter((git) => git.sub === 'commit' || isPr(git));
  if (guarded.length > 0 && gits.some((git) => git.sub === 'cd')) {
    return {
      block: true,
      reason: 'Do not combine `cd` with `git commit` or `gh pr`. Use `git -C <dir>` and run `gh` from the repository root.',
    };
  }
  for (const git of gits) {
    const out = (args) => {
      const result = run('git', args, git.dir);
      return result.status === 0 ? result.stdout.trim() : undefined;
    };

    if (isPr(git)) {
      const { action, args: prArgs } = prOf(git);
      if (action === 'merge') {
        return { block: true, reason: 'The maintainer merges pull requests. Do not run `gh pr merge` or enable auto-merge.' };
      }
      if (gits.length > 1) {
        return { block: true, reason: `Run \`gh pr ${action}\` as its own command.` };
      }
      const body = prBody(prArgs);
      if (body.problem || (action === 'create' && !body.file)) {
        return {
          block: true,
          reason: `Pass the PR description with --body-file, so that this hook can match it with the reviewer approvals${body.problem ? `; ${body.problem}` : ''}.`,
        };
      }
      const tree = out(['rev-parse', 'HEAD^{tree}']);
      const commonDir = out(['rev-parse', '--git-common-dir']);
      if (!tree || !commonDir) {
        continue;
      }
      const common = resolve(git.dir, commonDir);
      let text;
      if (body.file) {
        try {
          text = read(resolve(git.dir, body.file));
        } catch {
          return { block: true, reason: `Cannot read the PR description file ${body.file}.` };
        }
      }
      // Each role approves the tree of HEAD and the exact PR description.
      const roles = ['convention-reviewer', 'writing-reviewer'];
      // planning-reviewer approves the tree of HEAD when the branch changes a planning
      // artifact. It does not review the PR description, so it needs no text stamp. If the
      // branch diff fails, the hook does not require it.
      const branchPaths = (out(['diff', '--name-only', 'origin/main...HEAD']) ?? '').split('\n');
      const planning = branchPaths.some((path) => PLANNING_PATH.test(path));
      const missing = roles.filter(
        (role) => !exists(stampPath(common, role, tree)) || (text !== undefined && !exists(textStampPath(common, role, text))),
      );
      if (planning && !exists(stampPath(common, 'planning-reviewer', tree))) {
        missing.push('planning-reviewer');
      }
      if (missing.length > 0) {
        return {
          block: true,
          reason:
            `No approval for this branch and this PR description from: ${missing.join(', ')}. ` +
            missing
              .map((role) =>
                role === 'planning-reviewer'
                  ? 'Run `node scripts/review.mjs planning-reviewer ...`'
                  : `Run \`node scripts/review.mjs ${role} ... --stamp-file <description-file>\``,
              )
              .join(', and ') +
            '. Pass the same file with --body-file after each prints `verdict: APPROVE`.',
        };
      }
    }

    if (git.sub === 'commit' && gits.length > 1) {
      return {
        block: true,
        reason: 'Run `git commit` as its own command, so that it acts on the state that this hook checks.',
      };
    }

    if (git.sub === 'commit') {
      const problem = commitContentProblem(git.args);
      if (problem) {
        return {
          block: true,
          reason: `This commit can contain changes that are not staged, because ${problem}. Stage the change, then commit without -a, -i, -o, -p, or paths.`,
        };
      }
      const tree = out(['write-tree']);
      const commonDir = out(['rev-parse', '--git-common-dir']);
      if (!tree || !commonDir) {
        continue;
      }
      // A message-only amend keeps the tree of HEAD and needs no new review.
      if (tree === out(['rev-parse', 'HEAD^{tree}'])) {
        continue;
      }
      const required = ['code-reviewer'];
      const staged = (out(['diff', '--cached', '--name-only']) ?? '').split('\n');
      if (staged.some((path) => MIGRATION_PATH.test(path))) {
        required.push('migration-reviewer');
      }
      const missing = required.filter((role) => !exists(stampPath(resolve(git.dir, commonDir), role, tree)));
      if (missing.length > 0) {
        return {
          block: true,
          reason:
            `No reviewer approval for the staged tree ${tree} from: ${missing.join(', ')}. ` +
            'Stage the change, run `node scripts/review.mjs <role> ...` for each role, ' +
            'and commit only after it prints `verdict: APPROVE`. See .agents/rules/review-roles.md.',
        };
      }
    }
  }
  return { block: false };
}

function main() {
  const input = JSON.parse(readFileSync(0, 'utf8'));
  const decision = decide(
    { command: input.tool_input?.command ?? '', cwd: input.cwd ?? process.cwd() },
    {
      run: (cmd, args, cwd) => spawnSync(cmd, args, { cwd, encoding: 'utf8' }),
      exists: existsSync,
      read: (path) => readFileSync(path, 'utf8'),
    },
  );
  if (decision.block) {
    process.stderr.write(`${decision.reason}\n`);
    process.exit(2);
  }
}

if (process.argv[1] === fileURLToPath(import.meta.url)) {
  main();
}
