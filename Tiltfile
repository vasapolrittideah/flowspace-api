v1alpha1.extension_repo(
    name='default',
    url='https://github.com/tilt-dev/tilt-extensions',
    ref='a8701c0aa6a77ddcf60e02647d3a4730931e5f50',
)
load('ext://helm_resource', 'helm_resource')

context = k8s_context()
if not context.startswith('k3d-'):
    fail('FlowSpace local development requires a k3d Kubernetes context.')
allow_k8s_contexts(context)

password_file = os.getenv('KEYCLOAK_ADMIN_PASSWORD_FILE')
if not password_file or not os.path.exists(password_file):
    fail('Set KEYCLOAK_ADMIN_PASSWORD_FILE to an existing file before running tilt up.')
password_file = os.path.realpath(password_file)
if password_file.startswith(os.getcwd() + '/'):
    fail('KEYCLOAK_ADMIN_PASSWORD_FILE must be outside the repository.')
password = str(read_file(password_file))
if not password or password != password.strip():
    fail('KEYCLOAK_ADMIN_PASSWORD_FILE must contain one password without surrounding whitespace.')

helm_resource(
    'keycloak',
    'oci://ghcr.io/codecentric/helm-charts/keycloakx',
    namespace='flowspace-local',
    deps=['deploy/overlays/local/keycloak-values.yaml'],
    flags=[
        '--version=7.2.2',
        '--values=deploy/overlays/local/keycloak-values.yaml',
        '--create-namespace',
        '--set-file=secrets.admin.stringData.password=%s' % password_file,
    ],
    port_forwards=[port_forward(8080, 8080, name='Keycloak admin', link_path='/auth/admin/')],
)
