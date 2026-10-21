import assert from 'node:assert/strict';
import { mkdirSync, mkdtempSync, rmSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import test from 'node:test';

import { brokenLocalLinks } from './check-local-links.mjs';

test('local link check reports only missing file targets', (t) => {
  const directory = mkdtempSync(join(tmpdir(), 'flowspace-links-'));
  t.after(() => rmSync(directory, { force: true, recursive: true }));
  mkdirSync(join(directory, 'docs'));
  writeFileSync(join(directory, 'docs', 'target.md'), '# Target\n');
  writeFileSync(join(directory, 'docs', 'target with space.md'), '# Target\n');
  const source = join(directory, 'README.md');
  const markdown = [
    '[existing](docs/target.md#section)',
    '[space](<docs/target%20with%20space.md>)',
    '[heading](#heading)',
    '[website](https://example.com)',
    '[missing](docs/missing.md)',
    '```markdown',
    '[example](docs/example.md)',
    '```',
  ].join('\n');

  assert.deepEqual(brokenLocalLinks(markdown, source), [{ line: 5, target: 'docs/missing.md' }]);
});
