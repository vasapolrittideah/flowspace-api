v1alpha1.extension_repo(
    name='default',
    url='https://github.com/tilt-dev/tilt-extensions',
    ref='a8701c0aa6a77ddcf60e02647d3a4730931e5f50',
)
load('ext://helm_resource', 'helm_resource')

context = k8s_context()
if context != 'k3d-flowspace':
    fail('FlowSpace local development requires the k3d-flowspace Kubernetes context.')
allow_k8s_contexts(context)

def read_password_file(variable):
    path = os.getenv(variable)
    if not path or not os.path.exists(path):
        fail('Set %s to an existing file before running tilt up.' % variable)
    path = os.path.realpath(path)
    if path.startswith(os.getcwd() + '/'):
        fail('%s must be outside the repository.' % variable)
    password = str(read_file(path))
    if not password or password != password.strip():
        fail('%s must contain one password without surrounding whitespace.' % variable)
    return path, password

keycloak_password_file, _ = read_password_file('KEYCLOAK_ADMIN_PASSWORD_FILE')
_, workspace_database_password = read_password_file('WORKSPACE_DATABASE_PASSWORD_FILE')

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
