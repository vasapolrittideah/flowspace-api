// Runs a review role with `codex exec` in a read-only sandbox that denies the secrets.
import { join } from 'node:path';

import { HOME_SECRETS, WORKSPACE_SECRETS } from '../review.mjs';

export const command = 'codex';

// The values of `model_reasoning_effort` in the Codex configuration reference.
export const efforts = ['low', 'medium', 'high', 'xhigh', 'max', 'ultra'];

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

export function args({ model, effort, report, prompt, resume, home }) {
  // `codex exec resume` keeps none of the session configuration, so both forms pass it all.
  const settings = ['-m', model, '-c', `model_reasoning_effort="${effort}"`, ...sandboxArgs(home)];
  if (resume) {
    return ['exec', 'resume', resume, ...settings, '-o', report, prompt];
  }
  return ['exec', ...settings, '-o', report, prompt];
}

export function sessionID(log) {
  return log.match(/^session id:\s*(\S+)/m)?.[1];
}
