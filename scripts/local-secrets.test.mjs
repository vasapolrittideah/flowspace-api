import assert from 'node:assert/strict';
import { spawnSync } from 'node:child_process';
import { createPrivateKey } from 'node:crypto';
import { chmodSync, existsSync, mkdirSync, mkdtempSync, readFileSync, rmSync, statSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import test from 'node:test';
import { fileURLToPath } from 'node:url';

const repository = fileURLToPath(new URL('../', import.meta.url));

test('local passwords and Identity keys are created safely and excluded from Git and Docker', () => {
  const temporary = mkdtempSync(join(tmpdir(), 'flowspace-local-secrets-'));
  try {
    const root = join(temporary, 'repository');
    mkdirSync(join(root, '.secrets'), { recursive: true });
    mkdirSync(join(root, '.secrets-other'));
    writeFileSync(join(root, 'Taskfile.yaml'), readFileSync(join(repository, 'Taskfile.yaml')));
    mkdirSync(join(root, 'scripts'));
    writeFileSync(join(root, 'scripts', 'setup-secrets.mjs'), readFileSync(join(repository, 'scripts', 'setup-secrets.mjs')));
    const passwords = ['keycloak-admin-password', 'workspace-database-password', 'identity-database-password', 'redpanda-bootstrap-password', 'identity-broker-password'];
    const keyNames = ['identity-code-verifier-key', 'identity-delivery-key'];
    let previous;
    let previousKeys;
    for (let run = 0; run < 2; run++) {
      const setup = spawnSync('task', ['secrets:setup'], { cwd: root, encoding: 'utf8' });
      assert.equal(setup.status, 0, setup.error?.message ?? setup.stderr);
      assert.equal(statSync(join(root, '.secrets')).mode & 0o777, 0o700);
      const values = passwords.map((name) => {
        const path = join(root, '.secrets', name);
        const value = readFileSync(path, 'utf8');
        assert.match(value, /^[a-f0-9]{64}$/);
        assert.equal(value.length, 64, 'Passwords must not contain a trailing newline');
        assert.equal(statSync(path).mode & 0o777, 0o600);
        assert.ok(!`${setup.stdout}${setup.stderr}`.includes(value), 'Setup must not print passwords');
        chmodSync(path, 0o644);
        return value;
      });
      if (previous) assert.deepEqual(values, previous, 'Setup must preserve existing passwords');
      previous = values;
      const keys = keyNames.map((name) => {
        const path = join(root, '.secrets', name);
        const value = readFileSync(path);
        assert.equal(value.length, 32);
        assert.equal(statSync(path).mode & 0o777, 0o600);
        chmodSync(path, 0o644);
        return value;
      });
      const signingPath = join(root, '.secrets', 'identity-signing-key.pem');
      const signingKey = readFileSync(signingPath);
      assert.equal(createPrivateKey(signingKey).asymmetricKeyType, 'ed25519');
      assert.equal(statSync(signingPath).mode & 0o777, 0o600);
      chmodSync(signingPath, 0o644);
      keys.push(signingKey);
      if (previousKeys) assert.deepEqual(keys, previousKeys, 'Setup must preserve existing keys');
      previousKeys = keys;
      chmodSync(join(root, '.secrets'), 0o755);
    }
    const source = readFileSync(join(repository, 'Tiltfile'), 'utf8');
    const helper = source.slice(source.indexOf('def read_secret_path('), source.indexOf('\nkeycloak_password_file'));
    const calls = source.slice(source.indexOf('keycloak_password_file, _ ='), source.indexOf('\nk8s_yaml('));
    writeFileSync(join(root, 'Tiltfile'), `${helper}\n${calls}\n`);
    const environment = { ...process.env };
    delete environment.KEYCLOAK_ADMIN_PASSWORD_FILE;
    delete environment.WORKSPACE_DATABASE_PASSWORD_FILE;
    delete environment.IDENTITY_DATABASE_PASSWORD_FILE;
    delete environment.IDENTITY_SIGNING_KEY_FILE;
    delete environment.IDENTITY_CODE_VERIFIER_KEY_FILE;
    delete environment.IDENTITY_DELIVERY_KEY_FILE;
    delete environment.REDPANDA_BOOTSTRAP_PASSWORD_FILE;
    delete environment.IDENTITY_BROKER_PASSWORD_FILE;
    const defaults = spawnSync('tilt', ['alpha', 'tiltfile-result', '--file', join(root, 'Tiltfile')], {
      cwd: root, env: environment, encoding: 'utf8',
    });
    assert.equal(defaults.status, 0, defaults.error?.message ?? defaults.stderr);
    writeFileSync(join(root, 'Tiltfile'), `${helper}\n_, password = read_password_file('FLOWSPACE_TEST_SECRET_FILE', '.secrets/password')\n`);

    const cases = [
      [join(root, '.secrets', 'password'), 'local-test-value', 0],
      [join(temporary, 'external-password'), 'local-test-value', 0],
      [join(root, 'password'), 'local-test-value', 5],
      [join(root, '.secrets-other', 'password'), 'local-test-value', 5],
      [join(root, '.secrets', 'empty'), '', 5],
      [join(root, '.secrets', 'newline'), 'local-test-value\n', 5],
    ];
    for (const [path, value, expected] of cases) {
      writeFileSync(path, value, { mode: 0o600 });
      const result = spawnSync('tilt', ['alpha', 'tiltfile-result', '--file', join(root, 'Tiltfile')], {
        cwd: root,
        env: { ...process.env, FLOWSPACE_TEST_SECRET_FILE: path },
        encoding: 'utf8',
      });
      assert.equal(result.status, expected, result.error?.message ?? result.stderr);
    }

    const ignored = spawnSync('git', ['check-ignore', '--quiet', '--no-index', '.secrets/password'], { cwd: repository });
    assert.equal(ignored.status, 0, 'Git must ignore local secret files');
    writeFileSync(join(root, '.dockerignore'), readFileSync(join(repository, '.dockerignore')));
    writeFileSync(join(root, 'Dockerfile'), 'FROM scratch\nCOPY . /\n');
    const output = join(temporary, 'image');
    const build = spawnSync('docker', ['build', '--output', `type=local,dest=${output}`, root], { encoding: 'utf8' });
    assert.equal(build.status, 0, build.error?.message ?? build.stderr);
    assert.ok(existsSync(join(output, 'password')), 'The test must copy a file from the build context');
    assert.ok(!existsSync(join(output, '.secrets')), 'Docker must exclude local secret files');
  } finally {
    rmSync(temporary, { recursive: true, force: true });
  }
});
