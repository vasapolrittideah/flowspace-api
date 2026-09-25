import { generateKeyPairSync, randomBytes } from 'node:crypto';
import { chmodSync, existsSync, mkdirSync, writeFileSync } from 'node:fs';

mkdirSync('.secrets', { recursive: true, mode: 0o700 });
chmodSync('.secrets', 0o700);
for (const name of ['keycloak-admin-password', 'workspace-database-password', 'identity-database-password', 'redpanda-bootstrap-password', 'identity-broker-password']) {
  const path = `.secrets/${name}`;
  if (!existsSync(path)) {
    writeFileSync(path, randomBytes(32).toString('hex'), { flag: 'wx', mode: 0o600 });
  }
  chmodSync(path, 0o600);
}

const signingKey = '.secrets/identity-signing-key.pem';
if (!existsSync(signingKey)) {
  const key = generateKeyPairSync('ed25519').privateKey.export({ format: 'pem', type: 'pkcs8' });
  writeFileSync(signingKey, key, { flag: 'wx', mode: 0o600 });
}
chmodSync(signingKey, 0o600);

for (const name of ['identity-code-verifier-key', 'identity-delivery-key']) {
  const path = `.secrets/${name}`;
  if (!existsSync(path)) {
    writeFileSync(path, randomBytes(32), { flag: 'wx', mode: 0o600 });
  }
  chmodSync(path, 0o600);
}
