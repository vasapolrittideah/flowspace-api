package domain

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"strings"
	"testing"

	"golang.org/x/crypto/argon2"
)

func TestPasswordPolicy(t *testing.T) {
	allowed := func(context.Context, string) (bool, error) { return false, nil }
	for _, password := range []string{"1234567", "abcdefgh", "ABCDEF12", strings.Repeat("e\u0301", 7) + "ZZ"} {
		if _, err := HashPassword(context.Background(), password, allowed); !errors.Is(err, ErrInvalidPassword) {
			t.Errorf("password policy accepted invalid input: %v", err)
		}
	}
	for _, password := range []string{"abcdefg1", "123456789012345", strings.Repeat("é", 15), strings.Repeat("x", 64)} {
		if _, err := HashPassword(context.Background(), password, allowed); err != nil {
			t.Errorf("password policy rejected valid input: %v", err)
		}
	}
}

func TestPasswordBlocklistUsesNFCAndFailsClosed(t *testing.T) {
	password := "e\u0301abcdef1"
	called := false
	blocked := func(_ context.Context, normalized string) (bool, error) {
		called = true
		if normalized != "éabcdef1" {
			t.Errorf("blocklist received %q", normalized)
		}
		return true, nil
	}
	if _, err := HashPassword(context.Background(), password, blocked); !errors.Is(err, ErrInvalidPassword) || !called {
		t.Fatalf("compromised password result = %v, callback called = %t", err, called)
	}
	if _, err := HashPassword(context.Background(), "password123", func(context.Context, string) (bool, error) {
		return false, nil
	}); !errors.Is(err, ErrInvalidPassword) {
		t.Fatalf("local blocklist result = %v", err)
	}
	if _, err := HashPassword(context.Background(), "abcdefg1", nil); !errors.Is(err, ErrPasswordCheckUnavailable) {
		t.Fatalf("missing blocklist result = %v", err)
	}
	if _, err := HashPassword(context.Background(), "abcdefg1", func(context.Context, string) (bool, error) {
		return false, errors.New("secret diagnostic")
	}); !errors.Is(err, ErrPasswordCheckUnavailable) || strings.Contains(err.Error(), "secret") {
		t.Fatalf("failed blocklist result = %v", err)
	}
}

func TestHashPasswordUsesArgon2idAndPerPasswordSalt(t *testing.T) {
	allowed := func(context.Context, string) (bool, error) { return false, nil }
	first, err := HashPassword(context.Background(), "e\u0301abcdef1", allowed)
	if err != nil {
		t.Fatal(err)
	}
	second, err := HashPassword(context.Background(), "e\u0301abcdef1", allowed)
	if err != nil {
		t.Fatal(err)
	}
	parts := strings.Split(first, "$")
	if len(parts) != 6 || parts[1] != "argon2id" || parts[2] != "v=19" || parts[3] != "m=19456,t=2,p=1" {
		t.Fatalf("unexpected hash format: %q", first)
	}
	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil || len(salt) != 16 {
		t.Fatalf("invalid salt: %v", err)
	}
	derived, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil || len(derived) != 32 {
		t.Fatalf("invalid hash: %v", err)
	}
	if !bytes.Equal(derived, argon2.IDKey([]byte("éabcdef1"), salt, 2, 19456, 1, 32)) {
		t.Fatal("hash does not match normalized password and salt")
	}
	if first == second {
		t.Fatal("two passwords reused one salt")
	}
}
