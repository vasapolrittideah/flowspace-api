import assert from 'node:assert/strict';
import { copyFileSync, mkdirSync, mkdtempSync, rmSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { spawnSync } from 'node:child_process';
import { test } from 'node:test';

test('TOML checks validate syntax and formatting in .codex', () => {
  const directory = mkdtempSync(join(tmpdir(), 'flowspace-toml-'));
  try {
    for (const file of ['Taskfile.yaml', '.taplo.toml']) {
      copyFileSync(file, join(directory, file));
    }
    mkdirSync(join(directory, '.codex', 'agents'), { recursive: true });
    const file = join(directory, '.codex', 'agents', 'check.toml');
    for (const [content, valid] of [
      ['name = "valid"\n', true],
      ['name = "first"\nname = "second"\n', false],
      ['name="valid"\n', false],
    ]) {
      writeFileSync(file, content);
      const result = spawnSync('task', ['toml:check'], { cwd: directory, encoding: 'utf8' });
      assert.ifError(result.error);
      assert.equal(result.status === 0, valid, result.stdout + result.stderr);
    }
  } finally {
    rmSync(directory, { recursive: true, force: true });
  }
});
