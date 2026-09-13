package config

import (
	"encoding/json"
	"fmt"
	"testing"
)

func TestLoad(t *testing.T) {
	t.Setenv("TEST_SECRET", "sensitive-value")

	config, err := Load[struct {
		Secret Secret `env:"TEST_SECRET,required"`
	}]()
	if err != nil {
		t.Fatal(err)
	}
	if string(config.Secret) != "sensitive-value" {
		t.Fatalf("Secret = %q", config.Secret)
	}
}

func TestSecretRedactsValue(t *testing.T) {
	secret := Secret("sensitive-value")
	for name, value := range map[string]string{
		"String":   secret.String(),
		"GoString": secret.GoString(),
		"Format":   fmt.Sprintf("%d", secret),
	} {
		if value != redacted {
			t.Errorf("%s = %q, want %q", name, value, redacted)
		}
	}

	encoded, err := json.Marshal(secret)
	if err != nil {
		t.Fatal(err)
	}
	if string(encoded) != `"[REDACTED]"` {
		t.Fatalf("JSON = %s", encoded)
	}
}
