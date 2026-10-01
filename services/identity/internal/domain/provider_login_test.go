package domain

import (
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"strings"
	"testing"
)

func TestNewProviderAttemptGeneratesSeparateSecrets(t *testing.T) {
	google, err := NewProviderAttempt(ProviderGoogle)
	if err != nil {
		t.Fatal(err)
	}
	github, err := NewProviderAttempt(ProviderGitHub)
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{google.AttemptToken, google.State, google.CodeVerifier, google.Nonce, github.AttemptToken, github.State, github.CodeVerifier} {
		decoded, err := base64.RawURLEncoding.DecodeString(secret)
		if err != nil || len(decoded) != 32 {
			t.Fatalf("secret has %d random bytes: %v", len(decoded), err)
		}
	}
	if google.AttemptToken == google.State || google.State == github.State || google.CodeVerifier == github.CodeVerifier {
		t.Fatal("attempt secrets are reused")
	}
	if github.Nonce != "" {
		t.Fatal("GitHub attempt has a nonce")
	}
	if _, err := NewProviderAttempt("okta"); !errors.Is(err, ErrInvalidProvider) {
		t.Fatalf("unknown provider error = %v", err)
	}
}

func TestProviderCodeChallengeUsesS256(t *testing.T) {
	// RFC 7636 appendix B.
	if got := ProviderCodeChallenge("dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk"); got != "E9Melhoa2OwvFrEMTJguCHaoeK1t8URWbuGJSstw-cM" {
		t.Fatalf("challenge = %s", got)
	}
}

func TestProviderSecretVerifierSeparatesPurposes(t *testing.T) {
	key := make([]byte, 32)
	token := ProviderSecretVerifier(key, ProviderSecretAttemptToken, "value")
	state := ProviderSecretVerifier(key, ProviderSecretState, "value")
	if token == state || token == sha256.Sum256([]byte("value")) {
		t.Fatal("verifier does not separate purpose or key")
	}
	if ProviderSecretVerifier(key, ProviderSecretAttemptToken, "value") != token {
		t.Fatal("verifier is not stable")
	}
}

func TestNewProviderHandoffCodeHas256RandomBits(t *testing.T) {
	first, err := NewProviderHandoffCode()
	if err != nil {
		t.Fatal(err)
	}
	second, err := NewProviderHandoffCode()
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := base64.RawURLEncoding.DecodeString(first)
	if err != nil || len(decoded) != 32 || first == second {
		t.Fatalf("handoff code has %d bytes: %v", len(decoded), err)
	}
}

func TestValidProviderSecretAcceptsOnlyIssuedFormat(t *testing.T) {
	issued, err := NewProviderHandoffCode()
	if err != nil {
		t.Fatal(err)
	}
	if !ValidProviderSecret(issued) || !ValidProviderSecret(strings.Repeat("A", 43)) {
		t.Fatal("issued secret rejected")
	}
	// A final "B" sets padding bits, so it is not a canonical 32-byte value.
	for _, value := range []string{"", issued[:42], issued + "A", strings.Repeat("A", 42) + "=", strings.Repeat("A", 42) + "+", strings.Repeat("A", 42) + "B"} {
		if ValidProviderSecret(value) {
			t.Fatalf("accepted %q", value)
		}
	}
}
