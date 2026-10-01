package domain

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"strings"
)

type Provider string

const (
	ProviderGoogle Provider = "google"
	ProviderGitHub Provider = "github"
)

type ProviderSecret string

const (
	ProviderSecretAttemptToken ProviderSecret = "provider-attempt-token"
	ProviderSecretState        ProviderSecret = "provider-state"
	ProviderSecretHandoffCode  ProviderSecret = "provider-handoff-code"
)

// ProviderAttemptSecrets holds the fresh proofs for one provider login attempt.
// Only Google attempts receive a nonce.
type ProviderAttemptSecrets struct {
	AttemptToken string
	State        string
	CodeVerifier string
	Nonce        string
}

func NewProviderAttempt(provider Provider) (ProviderAttemptSecrets, error) {
	if provider != ProviderGoogle && provider != ProviderGitHub {
		return ProviderAttemptSecrets{}, ErrInvalidProvider
	}
	var secrets ProviderAttemptSecrets
	targets := []*string{&secrets.AttemptToken, &secrets.State, &secrets.CodeVerifier}
	if provider == ProviderGoogle {
		targets = append(targets, &secrets.Nonce)
	}
	for _, target := range targets {
		value, err := NewProviderHandoffCode()
		if err != nil {
			return ProviderAttemptSecrets{}, err
		}
		*target = value
	}
	return secrets, nil
}

// NewProviderHandoffCode returns a one-time code with 256 random bits. Attempt
// secrets use the same format.
func NewProviderHandoffCode() (string, error) {
	random := make([]byte, 32)
	if _, err := rand.Read(random); err != nil {
		return "", ErrCodeGenerationUnavailable
	}
	return base64.RawURLEncoding.EncodeToString(random), nil
}

// ValidProviderSecret reports whether value has the canonical format of an
// issued attempt token or handoff code.
func ValidProviderSecret(value string) bool {
	decoded, err := base64.RawURLEncoding.Strict().DecodeString(value)
	return err == nil && len(decoded) == 32
}

// NewProviderAccountEmail returns the normalized email for a new provider-only
// account and whether that account starts verified. It reports false when the
// provider gave no usable verified email. Only Google can prove current
// mailbox control: for a Gmail address, or a Workspace address with an hd claim.
func NewProviderAccountEmail(provider Provider, email string, verified bool, hostedDomain string) (string, bool, bool) {
	if !verified {
		return "", false, false
	}
	normalized, err := NormalizeEmail(email)
	if err != nil {
		return "", false, false
	}
	startsVerified := provider == ProviderGoogle && (strings.HasSuffix(normalized, "@gmail.com") || hostedDomain != "")
	return normalized, startsVerified, true
}

// ProviderCodeChallenge returns the PKCE S256 challenge for a code verifier.
func ProviderCodeChallenge(verifier string) string {
	digest := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(digest[:])
}

func ProviderSecretVerifier(key []byte, purpose ProviderSecret, value string) [32]byte {
	mac := hmac.New(sha256.New, key)
	for _, field := range []string{string(purpose), value} {
		_, _ = fmt.Fprintf(mac, "%d:%s", len(field), field)
	}
	var verifier [32]byte
	copy(verifier[:], mac.Sum(nil))
	return verifier
}
