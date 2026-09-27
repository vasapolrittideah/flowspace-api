package bootstrap

import (
	"errors"
	"os"

	sharedconfig "github.com/vasapolrittideah/flowspace-api/internal/config"
)

type Config struct {
	Environment               string              `env:"ENVIRONMENT,required,notEmpty"`
	HTTPAddress               string              `env:"HTTP_ADDR"                                      envDefault:":8080"`
	DatabaseURL               sharedconfig.Secret `env:"DATABASE_URL"`
	IdentityJWKSURL           string              `env:"IDENTITY_JWKS_URL,required,notEmpty"`
	IdentityIssuer            string              `env:"IDENTITY_ISSUER,required,notEmpty"`
	IdentityAudience          string              `env:"IDENTITY_AUDIENCE,required,notEmpty"`
	IdentitySessionAddress    string              `env:"IDENTITY_SESSION_ADDR,required,notEmpty"`
	IdentitySessionServerName string              `env:"IDENTITY_SESSION_SERVER_NAME,required,notEmpty"`
	IdentityClientCertFile    string              `env:"IDENTITY_CLIENT_CERT_FILE,required,notEmpty"`
	IdentityClientKeyFile     string              `env:"IDENTITY_CLIENT_KEY_FILE,required,notEmpty"`
	IdentityCAFile            string              `env:"IDENTITY_CA_FILE,required,notEmpty"`
	OIDCDiscoveryURL          string              `env:"OIDC_DISCOVERY_URL"`
	OIDCIssuer                string              `env:"OIDC_ISSUER"`
	OIDCAudience              string              `env:"OIDC_AUDIENCE"`
}

func LoadConfig() (Config, error) {
	config, err := sharedconfig.Load[Config]()
	if err != nil {
		return Config{}, err
	}
	if config.DatabaseURL == "" && !postgresEnvironmentConfigured() {
		return Config{}, errors.New("DATABASE_URL or PGHOST, PGDATABASE, PGUSER, and PGPASSWORD are required")
	}
	if config.OIDCDiscoveryURL == "" {
		config.OIDCDiscoveryURL = config.OIDCIssuer
	}
	if config.Environment == "local" && (config.IdentityIssuer != "urn:flowspace:identity:local" || config.IdentityAudience != "flowspace-api") {
		return Config{}, errors.New("invalid local Identity token configuration")
	}
	return config, nil
}

func postgresEnvironmentConfigured() bool {
	for _, key := range []string{"PGHOST", "PGDATABASE", "PGUSER", "PGPASSWORD"} {
		if os.Getenv(key) == "" {
			return false
		}
	}
	return true
}
