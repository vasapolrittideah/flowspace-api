// Package config loads typed application configuration from environment variables.
package config

import "github.com/caarlos0/env/v11"

// Load parses environment variables into T.
func Load[T any]() (T, error) {
	return env.ParseAs[T]()
}
