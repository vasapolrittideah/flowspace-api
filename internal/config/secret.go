package config

import "fmt"

const redacted = "[REDACTED]"

// Secret is a string that redacts itself when formatted or encoded as text.
type Secret string

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
