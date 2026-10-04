// Claude Code PreToolUse hook for Bash. It blocks a `git commit` that has no Codex approval
// for the staged tree, and a `git push` to a branch whose pull request is merged or closed.
// It guards against forgotten steps, not against a deliberate bypass.
import { spawnSync } from 'node:child_process';
import { existsSync, readFileSync } from 'node:fs';
import { join, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';

import { stampPath } from './codex-review.mjs';

const MIGRATION_PATH = /^services\/[^/]+\/db\/migrations\//;
const WRAPPERS = new Set(['command', 'exec', 'env', 'time', 'nohup', 'builtin']);
const COMMIT_VALUE_OPTIONS = new Set(['-m', '-F', '-C', '-c', '-t', '--message', '--file', '--reuse-message',
  '--reedit-message', '--template', '--author', '--date', '--fixup', '--squash', '--trailer', '--cleanup']);
const COMMIT_CONTENT_OPTIONS = new Set(['-a', '--all', '-i', '--include', '-o', '--only', '-p', '--patch',
  '--interactive', '--pathspec-from-file']);
const PUSH_VALUE_OPTIONS = new Set(['-o', '--push-option', '--receive-pack', '--exec']);

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
// so `decide` rejects a commit or a push that shares its line with `cd`.
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

// Returns the branches that a `git push` sends. `all` is true for a push that names no
// branch but sends many, such as --all, --mirror, or the matching refspec `:`.
export function pushTargets(args, currentBranch) {
  let remote;
  let repoOption = false;
  let all = false;
  let deletes = false;
  const refspecs = [];
  for (let i = 0; i < args.length; i += 1) {
    const arg = args[i];
    if (arg === '--repo') {
      repoOption = true;
      i += 1;
    } else if (arg.startsWith('--repo=')) {
      repoOption = true;
    } else if (PUSH_VALUE_OPTIONS.has(arg)) {
      i += 1;
    } else if (arg === '--all' || arg === '--mirror' || arg === '--branches') {
      all = true;
    } else if (arg === '--delete' || arg === '-d') {
      deletes = true;
    } else if (!arg.startsWith('-')) {
      if (!repoOption && remote === undefined) {
        remote = arg;
      } else {
        refspecs.push(arg);
      }
    }
  }
  if (refspecs.includes(':') || refspecs.includes('+:')) {
    all = true;
  }
  if (refspecs.length === 0) {
    return { all, deletes, branches: currentBranch ? [currentBranch] : [] };
  }
  const branches = [];
  for (const refspec of refspecs) {
    if (refspec.startsWith(':')) {
      continue;
    }
    let target = refspec.includes(':') ? refspec.split(':')[1] : refspec;
    target = target.replace(/^\+/, '').replace(/^refs\/heads\//, '');
    if (target === 'HEAD') {
      target = currentBranch;
    }
    if (target && !target.startsWith('refs/tags/')) {
      branches.push(target);
    }
  }
  return { all, deletes, branches };
}

// Decides whether to block the command. `run(cmd, args, cwd)` returns { status, stdout }.
export function decide({ command, cwd }, { run, exists }) {
  const gits = gitCommands(command, cwd);
  const guarded = gits.filter((git) => git.sub === 'commit' || git.sub === 'push');
  if (guarded.length > 0 && gits.some((git) => git.sub === 'cd')) {
    return {
      block: true,
      reason: 'Do not combine `cd` with `git commit` or `git push`. Use `git -C <dir>` so that this hook checks the right repository.',
    };
  }
  for (const git of gits) {
    const out = (args) => {
      const result = run('git', args, git.dir);
      return result.status === 0 ? result.stdout.trim() : undefined;
    };

    if ((git.sub === 'commit' || git.sub === 'push') && gits.length > 1) {
      return {
        block: true,
        reason: `Run \`git ${git.sub}\` as its own command, so that it acts on the state that this hook checks.`,
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
      const root = out(['rev-parse', '--show-toplevel']) ?? git.dir;
      if (
        staged.some((path) => MIGRATION_PATH.test(path)) &&
        exists(join(root, '.codex/agents/migration-reviewer.toml'))
      ) {
        required.push('migration-reviewer');
      }
      const missing = required.filter((role) => !exists(stampPath(resolve(git.dir, commonDir), role, tree)));
      if (missing.length > 0) {
        return {
          block: true,
          reason:
            `No Codex approval for the staged tree ${tree} from: ${missing.join(', ')}. ` +
            'Stage the change, run `node scripts/codex-review.mjs <role> ...` for each role, ' +
            'and commit only after it prints `verdict: APPROVE`. See .claude/rules/codex-review-roles.md.',
        };
      }
    }

    if (git.sub === 'push') {
      const targets = pushTargets(git.args, out(['rev-parse', '--abbrev-ref', 'HEAD']));
      if (targets.all) {
        return { block: true, reason: 'Push branches by name, so that this hook can check their pull requests.' };
      }
      if (targets.deletes) {
        continue;
      }
      for (const branch of targets.branches) {
        const result = run('gh', ['pr', 'list', '--head', branch, '--state', 'all', '--json', 'number,state'], git.dir);
        if (result.status !== 0) {
          continue;
        }
        const prs = JSON.parse(result.stdout || '[]');
        if (prs.length > 0 && !prs.some((pr) => pr.state === 'OPEN')) {
          const list = prs.map((pr) => `#${pr.number} ${pr.state}`).join(', ');
          return {
            block: true,
            reason:
              `The pull request of branch ${branch} is not open (${list}). ` +
              'Commits pushed now never reach main. Move them to a new branch from main and open a new PR.',
          };
        }
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
