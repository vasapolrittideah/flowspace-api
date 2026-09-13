// Package config loads typed application configuration from environment variables.
package config

import (
	"fmt"

	"github.com/caarlos0/env/v11"
)

const redacted = "[REDACTED]"

// Secret is a string that redacts itself when formatted or encoded as text.
type Secret string

// Load parses environment variables into T.
func Load[T any]() (T, error) {
	return env.ParseAs[T]()
}

func (Secret) String() string {
	return redacted
}

func (Secret) GoString() string {
	return redacted
}

func (Secret) Format(state fmt.State, _ rune) {
	_, _ = state.Write([]byte(redacted))
}

func (Secret) MarshalText() ([]byte, error) {
	return []byte(redacted), nil
}
