package bootstrap

import (
	"errors"
	"os"
)

type Config struct {
	HTTPAddress  string
	DatabaseURL  string
	OIDCIssuer   string
	OIDCAudience string
}

func LoadConfig() (Config, error) {
	config := Config{
		HTTPAddress:  os.Getenv("HTTP_ADDR"),
		DatabaseURL:  os.Getenv("DATABASE_URL"),
		OIDCIssuer:   os.Getenv("OIDC_ISSUER"),
		OIDCAudience: os.Getenv("OIDC_AUDIENCE"),
	}
	if config.HTTPAddress == "" {
		config.HTTPAddress = ":8080"
	}
	if config.DatabaseURL == "" {
		return Config{}, errors.New("DATABASE_URL is required")
	}
	if config.OIDCIssuer == "" {
		return Config{}, errors.New("OIDC_ISSUER is required")
	}
	if config.OIDCAudience == "" {
		return Config{}, errors.New("OIDC_AUDIENCE is required")
	}
	return config, nil
}
