// Runs a review role with `claude -p`. The reviewer gets only the Read, Grep, Glob, and Bash
// tools. Permission rules deny the secrets to the file tools, and the operating system sandbox
// denies them to Bash and blocks writes to the repository.
import { join } from 'node:path';

import { HOME_SECRETS, WORKSPACE_SECRETS } from '../review.mjs';

export const command = 'claude';

export function settings({ home, root, gitDirs = [] }) {
  const homePaths = HOME_SECRETS.map((path) => join(home, path));
  // A permission rule that starts with `//` is an absolute path. A rule without a leading
  // slash matches under the working directory, like a .gitignore pattern.
  const readRules = [
    ...homePaths.flatMap((path) => [`Read(/${path})`, `Read(/${path}/**)`]),
    ...WORKSPACE_SECRETS.map((pattern) => `Read(${pattern})`),
  ];
  return {
    permissions: { deny: readRules },
    sandbox: {
      enabled: true,
      // Without the sandbox, Bash can read the secrets, so the review must not start.
      failIfUnavailable: true,
      allowUnsandboxedCommands: false,
      autoAllowBashIfSandboxed: true,
      filesystem: {
        denyRead: [...homePaths, ...WORKSPACE_SECRETS.map((pattern) => join(root, pattern))],
        // A linked worktree keeps its Git directories outside the root. A write there can add
        // an approval stamp or move a ref, so the reviewer must not write to them either.
        denyWrite: [root, ...gitDirs],
      },
    },
  };
}

export function args({ model, effort, prompt, resume, home, root, gitDirs }) {
  // --restricted ignores the user and project configuration files, so their hooks and allow rules
  // cannot widen the review. A resumed session needs all options again.
  const options = [
    '-p',
    prompt,
    '--model',
    model,
    '--effort',
    effort,
    '--output-format',
    'json',
    '--restricted',
    '--strict-mcp-config',
    '--tools',
    'Read,Grep,Glob,Bash',
    '--permission-mode',
    'dontAsk',
    '--settings',
    JSON.stringify(settings({ home, root, gitDirs })),
  ];
  return resume ? [...options, '--resume', resume] : options;
}

// Returns the JSON result object that `claude -p --output-format json` prints on one line.
function result(log) {
  for (const line of log.split('\n').reverse()) {
    try {
      const value = JSON.parse(line);
      if (value?.type === 'result') {
        return value;
      }
    } catch {
      // Other lines are diagnostics.
    }
  }
  return undefined;
}

export function sessionID(log) {
  return result(log)?.session_id;
}

// An error result holds an error message, not a review, so it gives an empty report.
export function report(log) {
  const value = result(log);
  return value && !value.is_error && typeof value.result === 'string' ? value.result : '';
}
