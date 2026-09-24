package domain

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"fmt"
	"io"
	"math/big"
	"time"
)

type CodePurpose string

const (
	PurposeVerifyEmail  CodePurpose = "verify-email"
	PurposeClaimAccount CodePurpose = "claim-account"
)

func newCode(reader io.Reader) (string, error) {
	n, err := rand.Int(reader, big.NewInt(1_000_000))
	if err != nil {
		return "", ErrCodeGenerationUnavailable
	}
	return fmt.Sprintf("%06d", n.Int64()), nil
}

func NewChallenge(key []byte, subject, address string, purpose CodePurpose, now time.Time) (string, [32]byte, time.Time, error) {
	if len(key) != 32 || subject == "" || !validPurpose(purpose) {
		return "", [32]byte{}, time.Time{}, ErrInvalidChallenge
	}
	email, err := NormalizeEmail(address)
	if err != nil {
		return "", [32]byte{}, time.Time{}, ErrInvalidChallenge
	}
	code, err := newCode(rand.Reader)
	if err != nil {
		return "", [32]byte{}, time.Time{}, err
	}
	return code, codeVerifier(key, subject, email, purpose, code), now.Add(10 * time.Minute), nil
}

func VerifyChallenge(key []byte, subject, address string, purpose CodePurpose, code string, expected [32]byte, expiresAt, now time.Time) bool {
	if len(key) != 32 || subject == "" || !validPurpose(purpose) || !now.Before(expiresAt) || len(code) != 6 {
		return false
	}
	for i := range len(code) {
		if code[i] < '0' || code[i] > '9' {
			return false
		}
	}
	email, err := NormalizeEmail(address)
	if err != nil {
		return false
	}
	verifier := codeVerifier(key, subject, email, purpose, code)
	return subtle.ConstantTimeCompare(verifier[:], expected[:]) == 1
}

func validPurpose(purpose CodePurpose) bool {
	return purpose == PurposeVerifyEmail || purpose == PurposeClaimAccount
}

func codeVerifier(key []byte, subject, email string, purpose CodePurpose, code string) [32]byte {
	mac := hmac.New(sha256.New, key)
	for _, field := range []string{subject, email, string(purpose), code} {
		_, _ = fmt.Fprintf(mac, "%d:%s", len(field), field)
	}
	var verifier [32]byte
	copy(verifier[:], mac.Sum(nil))
	return verifier
}
