package bootstrap

import "testing"

func TestLoadConfig(t *testing.T) {
	setIdentityEnv(t)
	t.Setenv("ENVIRONMENT", "local")
	t.Setenv("DATABASE_URL", "postgres://workspace")
	t.Setenv("OIDC_ISSUER", "https://identity.test/realms/flowspace")
	t.Setenv("OIDC_DISCOVERY_URL", "http://keycloak/realms/flowspace")
	t.Setenv("OIDC_AUDIENCE", "workspace-api")
	t.Setenv("HTTP_ADDR", "")

	config, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}
	if config.Environment != "local" || config.HTTPAddress != ":8080" || config.DatabaseURL != "postgres://workspace" || config.OIDCIssuer != "https://identity.test/realms/flowspace" || config.OIDCDiscoveryURL != "http://keycloak/realms/flowspace" || config.OIDCAudience != "workspace-api" {
		t.Fatalf("config = %+v", config)
	}
}

func TestLoadConfigUsesIssuerForDiscoveryByDefault(t *testing.T) {
	setIdentityEnv(t)
	t.Setenv("ENVIRONMENT", "local")
	t.Setenv("DATABASE_URL", "postgres://workspace")
	t.Setenv("OIDC_ISSUER", "https://identity.test/realms/flowspace")
	t.Setenv("OIDC_DISCOVERY_URL", "")
	t.Setenv("OIDC_AUDIENCE", "workspace-api")

	config, err := LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if config.OIDCDiscoveryURL != config.OIDCIssuer {
		t.Fatalf("OIDCDiscoveryURL = %q, want %q", config.OIDCDiscoveryURL, config.OIDCIssuer)
	}
}

func TestLoadConfigAcceptsPostgresEnvironment(t *testing.T) {
	setIdentityEnv(t)
	t.Setenv("ENVIRONMENT", "local")
	t.Setenv("DATABASE_URL", "")
	t.Setenv("PGHOST", "workspace-postgres")
	t.Setenv("PGDATABASE", "workspace")
	t.Setenv("PGUSER", "workspace")
	t.Setenv("PGPASSWORD", "secret")
	t.Setenv("OIDC_ISSUER", "https://identity.test/realms/flowspace")
	t.Setenv("OIDC_AUDIENCE", "workspace-api")

	if _, err := LoadConfig(); err != nil {
		t.Fatal(err)
	}
}

func TestLoadConfigRequiresDependencies(t *testing.T) {
	tests := []string{"ENVIRONMENT", "DATABASE_URL", "IDENTITY_ISSUER", "IDENTITY_AUDIENCE", "IDENTITY_JWKS_URL", "IDENTITY_SESSION_ADDR", "IDENTITY_SESSION_SERVER_NAME", "IDENTITY_CLIENT_CERT_FILE", "IDENTITY_CLIENT_KEY_FILE", "IDENTITY_CA_FILE"}
	for _, missing := range tests {
		t.Run(missing, func(t *testing.T) {
			setIdentityEnv(t)
			t.Setenv("ENVIRONMENT", "local")
			t.Setenv("DATABASE_URL", "postgres://workspace")
			for _, key := range []string{"PGHOST", "PGDATABASE", "PGUSER", "PGPASSWORD"} {
				t.Setenv(key, "")
			}
			t.Setenv("OIDC_ISSUER", "https://identity.test/realms/flowspace")
			t.Setenv("OIDC_AUDIENCE", "workspace-api")
			t.Setenv(missing, "")

			if _, err := LoadConfig(); err == nil {
				t.Fatalf("LoadConfig() accepted missing %s", missing)
			}
		})
	}
}

func TestLoadConfigRejectsWrongLocalIssuer(t *testing.T) {
	setIdentityEnv(t)
	t.Setenv("ENVIRONMENT", "local")
	t.Setenv("DATABASE_URL", "postgres://workspace")
	t.Setenv("IDENTITY_ISSUER", "urn:wrong")
	if _, err := LoadConfig(); err == nil {
		t.Fatal("LoadConfig() accepted wrong local issuer")
	}
}

func setIdentityEnv(t *testing.T) {
	t.Helper()
	t.Setenv("IDENTITY_ISSUER", "urn:flowspace:identity:local")
	t.Setenv("IDENTITY_AUDIENCE", "flowspace-api")
	t.Setenv("IDENTITY_JWKS_URL", "http://identity-internal/.well-known/jwks.json")
	t.Setenv("IDENTITY_SESSION_ADDR", "identity-session:8082")
	t.Setenv("IDENTITY_SESSION_SERVER_NAME", "identity-session.flowspace-local.svc")
	t.Setenv("IDENTITY_CLIENT_CERT_FILE", "/keys/session/tls.crt")
	t.Setenv("IDENTITY_CLIENT_KEY_FILE", "/keys/session/tls.key")
	t.Setenv("IDENTITY_CA_FILE", "/keys/session/ca.crt")
}
