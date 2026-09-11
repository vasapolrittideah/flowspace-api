package bootstrap

import (
	"errors"
	"os"
)

type Config struct {
	HTTPAddress      string
	DatabaseURL      string
	OIDCDiscoveryURL string
	OIDCIssuer       string
	OIDCAudience     string
}

func LoadConfig() (Config, error) {
	config := Config{
		HTTPAddress:      os.Getenv("HTTP_ADDR"),
		DatabaseURL:      os.Getenv("DATABASE_URL"),
		OIDCDiscoveryURL: os.Getenv("OIDC_DISCOVERY_URL"),
		OIDCIssuer:       os.Getenv("OIDC_ISSUER"),
		OIDCAudience:     os.Getenv("OIDC_AUDIENCE"),
	}
	if config.HTTPAddress == "" {
		config.HTTPAddress = ":8080"
	}
	if config.DatabaseURL == "" && !postgresEnvironmentConfigured() {
		return Config{}, errors.New("DATABASE_URL or PGHOST, PGDATABASE, PGUSER, and PGPASSWORD are required")
	}
	if config.OIDCIssuer == "" {
		return Config{}, errors.New("OIDC_ISSUER is required")
	}
	if config.OIDCDiscoveryURL == "" {
		config.OIDCDiscoveryURL = config.OIDCIssuer
	}
	if config.OIDCAudience == "" {
		return Config{}, errors.New("OIDC_AUDIENCE is required")
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
