import { randomBytes } from 'node:crypto';
import { chmodSync, existsSync, mkdirSync, writeFileSync } from 'node:fs';

mkdirSync('.secrets', { recursive: true, mode: 0o700 });
chmodSync('.secrets', 0o700);
for (const name of ['keycloak-admin-password', 'workspace-database-password']) {
  const path = `.secrets/${name}`;
  if (!existsSync(path)) {
    writeFileSync(path, randomBytes(32).toString('hex'), { flag: 'wx', mode: 0o600 });
  }
  chmodSync(path, 0o600);
}
