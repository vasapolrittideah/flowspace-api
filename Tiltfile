v1alpha1.extension_repo(
    name='default',
    url='https://github.com/tilt-dev/tilt-extensions',
    ref='a8701c0aa6a77ddcf60e02647d3a4730931e5f50',
)
load('ext://helm_resource', 'helm_resource', 'helm_repo')

context = k8s_context()
if context != 'k3d-flowspace':
    fail('FlowSpace local development requires the k3d-flowspace Kubernetes context.')
allow_k8s_contexts(context)
update_settings(k8s_upsert_timeout_secs=300)

def read_secret_path(variable, default):
    path = os.getenv(variable, default)
    if not path or not os.path.exists(path):
        fail('Run task secrets:setup or set %s to an existing secret file.' % variable)
    path = os.path.realpath(path)
    repository = os.getcwd() + '/'
    if path.startswith(repository) and not path.startswith(repository + '.secrets/'):
        fail('%s must be inside .secrets/ or outside the repository.' % variable)
    watch_file(path)
    return path

def read_password_file(variable, default):
    path = read_secret_path(variable, default)
    password = str(read_file(path))
    if not password or password != password.strip():
        fail('%s must contain one password without surrounding whitespace.' % variable)
    return path, password

def read_binary_key(variable, default):
    path = read_secret_path(variable, default)
    encoded = str(local(['base64', '-i', path], quiet=True, echo_off=True)).strip()
    if len(encoded) != 44:
        fail('%s must contain a 32-byte key.' % variable)
    return encoded

keycloak_password_file, _ = read_password_file('KEYCLOAK_ADMIN_PASSWORD_FILE', '.secrets/keycloak-admin-password')
_, workspace_database_password = read_password_file('WORKSPACE_DATABASE_PASSWORD_FILE', '.secrets/workspace-database-password')
_, identity_database_password = read_password_file('IDENTITY_DATABASE_PASSWORD_FILE', '.secrets/identity-database-password')
_, redpanda_bootstrap_password = read_password_file('REDPANDA_BOOTSTRAP_PASSWORD_FILE', '.secrets/redpanda-bootstrap-password')
_, identity_broker_password = read_password_file('IDENTITY_BROKER_PASSWORD_FILE', '.secrets/identity-broker-password')
identity_signing_key_file = read_secret_path('IDENTITY_SIGNING_KEY_FILE', '.secrets/identity-signing-key.pem')
identity_code_key = read_binary_key('IDENTITY_CODE_VERIFIER_KEY_FILE', '.secrets/identity-code-verifier-key')
identity_delivery_key = read_binary_key('IDENTITY_DELIVERY_KEY_FILE', '.secrets/identity-delivery-key')

k8s_yaml(encode_yaml({
    'apiVersion': 'v1',
    'kind': 'Secret',
    'metadata': {
        'name': 'workspace-database',
        'namespace': 'flowspace-local',
    },
    'stringData': {
        'PGPASSWORD': workspace_database_password,
    },
}))

k8s_yaml(encode_yaml({
    'apiVersion': 'v1',
    'kind': 'Secret',
    'metadata': {'name': 'identity-database', 'namespace': 'flowspace-local'},
    'stringData': {
        'POSTGRES_PASSWORD': identity_database_password,
        'DATABASE_URL': 'postgres://identity:%s@identity-postgres:5432/identity?sslmode=disable' % identity_database_password,
    },
}))
k8s_yaml(encode_yaml({
    'apiVersion': 'v1',
    'kind': 'Secret',
    'metadata': {'name': 'identity-signing-key', 'namespace': 'flowspace-local'},
    'stringData': {'private.pem': str(read_file(identity_signing_key_file))},
}))
k8s_yaml(encode_yaml({
    'apiVersion': 'v1',
    'kind': 'Secret',
    'metadata': {'name': 'identity-code-key', 'namespace': 'flowspace-local'},
    'data': {'key': identity_code_key},
}))
k8s_yaml(encode_yaml({
    'apiVersion': 'v1',
    'kind': 'Secret',
    'metadata': {'name': 'identity-delivery-key', 'namespace': 'flowspace-local'},
    'data': {'key': identity_delivery_key},
}))
k8s_yaml(encode_yaml({
    'apiVersion': 'v1',
    'kind': 'Secret',
    'metadata': {'name': 'redpanda-bootstrap-user', 'namespace': 'flowspace-local'},
    'stringData': {'password': redpanda_bootstrap_password},
}))
k8s_yaml(encode_yaml({
    'apiVersion': 'v1',
    'kind': 'Secret',
    'metadata': {'name': 'redpanda-superusers', 'namespace': 'flowspace-local'},
    'stringData': {'superusers.txt': ''},
}))
k8s_yaml(encode_yaml({
    'apiVersion': 'v1',
    'kind': 'Secret',
    'metadata': {'name': 'identity-broker', 'namespace': 'flowspace-local'},
    'stringData': {'BROKER_PASSWORD': identity_broker_password},
}))

helm_resource(
    'keycloak',
    'oci://ghcr.io/codecentric/helm-charts/keycloakx',
    namespace='flowspace-local',
    deps=['deploy/overlays/local/keycloak-values.yaml'],
    flags=[
        '--version=7.2.2',
        '--values=deploy/overlays/local/keycloak-values.yaml',
        '--create-namespace',
        '--set-file=secrets.admin.stringData.password=%s' % keycloak_password_file,
        '--set-file=secrets.realm.stringData.realm=deploy/overlays/local/flowspace-realm.json',
    ],
    port_forwards=[port_forward(8080, 8080, name='Keycloak admin', link_path='/auth/admin/')],
)

docker_build(
    'flowspace/workspace-api',
    '.',
    dockerfile='services/workspace/Dockerfile',
    only=['go.mod', 'go.sum', 'gen', 'services/workspace'],
)
k8s_yaml(kustomize('deploy/overlays/local/workspace'))
k8s_resource('workspace-migrate', resource_deps=['workspace-postgres'])
k8s_resource(
    'workspace-api',
    resource_deps=['keycloak', 'workspace-migrate'],
    port_forwards=[port_forward(8081, 8080, name='Workspace API')],
)

docker_build(
    'flowspace/identity-api',
    '.',
    dockerfile='services/identity/Dockerfile',
    only=['go.mod', 'go.sum', 'contracts/events', 'gen', 'internal', 'services/identity'],
)
k8s_yaml(kustomize('deploy/overlays/local/identity'))
k8s_resource(
    new_name='identity-secrets',
    objects=['identity-database:secret', 'identity-signing-key:secret', 'identity-code-key:secret', 'identity-delivery-key:secret', 'redpanda-bootstrap-user:secret', 'redpanda-superusers:secret', 'identity-broker:secret'],
)
helm_repo('redpanda', 'https://charts.redpanda.com', resource_name='redpanda-chart-repo')
helm_resource(
    'redpanda',
    'redpanda/redpanda',
    namespace='flowspace-local',
    deps=['deploy/overlays/local/redpanda-values.yaml'],
    flags=['--version=26.2.4', '--values=deploy/overlays/local/redpanda-values.yaml', '--create-namespace'],
    resource_deps=['redpanda-chart-repo', 'identity-secrets'],
)
helm_resource(
    'mailpit',
    'oci://ghcr.io/jouve/charts/mailpit',
    namespace='flowspace-local',
    deps=['deploy/overlays/local/mailpit-values.yaml'],
    flags=['--version=0.36.0', '--values=deploy/overlays/local/mailpit-values.yaml', '--create-namespace'],
)
k8s_resource('identity-postgres', resource_deps=['identity-secrets'])
k8s_resource('identity-migrate', resource_deps=['identity-postgres'])
k8s_resource('identity-broker-bootstrap', resource_deps=['redpanda', 'identity-secrets'])
k8s_resource(
    'identity-api',
    resource_deps=['identity-migrate', 'identity-secrets'],
    port_forwards=[port_forward(8082, 8080, name='Identity API')],
)
k8s_resource('identity-worker', resource_deps=['identity-migrate', 'identity-secrets', 'identity-broker-bootstrap', 'mailpit'])
