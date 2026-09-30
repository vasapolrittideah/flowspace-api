package domain

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
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
		random := make([]byte, 32)
		if _, err := rand.Read(random); err != nil {
			return ProviderAttemptSecrets{}, ErrCodeGenerationUnavailable
		}
		*target = base64.RawURLEncoding.EncodeToString(random)
	}
	return secrets, nil
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
