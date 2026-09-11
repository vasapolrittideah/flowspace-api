package bootstrap

import "testing"

func TestLoadConfig(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://workspace")
	t.Setenv("OIDC_ISSUER", "https://identity.test/realms/flowspace")
	t.Setenv("OIDC_DISCOVERY_URL", "http://keycloak/realms/flowspace")
	t.Setenv("OIDC_AUDIENCE", "workspace-api")
	t.Setenv("HTTP_ADDR", "")

	config, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}
	if config.HTTPAddress != ":8080" || config.DatabaseURL != "postgres://workspace" || config.OIDCIssuer != "https://identity.test/realms/flowspace" || config.OIDCDiscoveryURL != "http://keycloak/realms/flowspace" || config.OIDCAudience != "workspace-api" {
		t.Fatalf("config = %+v", config)
	}
}

func TestLoadConfigUsesIssuerForDiscoveryByDefault(t *testing.T) {
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
	tests := []string{"DATABASE_URL", "OIDC_ISSUER", "OIDC_AUDIENCE"}
	for _, missing := range tests {
		t.Run(missing, func(t *testing.T) {
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
