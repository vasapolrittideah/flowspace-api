package bootstrap

import (
	"errors"
	"os"

	sharedconfig "github.com/vasapolrittideah/flowspace-api/internal/config"
)

type Config struct {
	HTTPAddress      string              `env:"HTTP_ADDR"                       envDefault:":8080"`
	DatabaseURL      sharedconfig.Secret `env:"DATABASE_URL"`
	OIDCDiscoveryURL string              `env:"OIDC_DISCOVERY_URL"`
	OIDCIssuer       string              `env:"OIDC_ISSUER,required,notEmpty"`
	OIDCAudience     string              `env:"OIDC_AUDIENCE,required,notEmpty"`
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
