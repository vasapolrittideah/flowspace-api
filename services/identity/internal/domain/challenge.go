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
	PurposeVerifyEmail    CodePurpose = "verify-email"
	PurposeClaimAccount   CodePurpose = "claim-account"
	PurposePasswordReset  CodePurpose = "password-reset"
	PurposeReauthenticate CodePurpose = "reauthenticate"
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

// Challenge is a stored code challenge. A zero Challenge rejects every code.
type Challenge struct {
	ID           string
	Email        string
	Verifier     [32]byte
	WrongGuesses int16
	ExpiresAt    time.Time
}

// CodeCheck is the outcome of checking a code against a Challenge.
type CodeCheck int

const (
	// CodeRejected means that the challenge cannot accept guesses, so the guess does not count.
	CodeRejected CodeCheck = iota
	// CodeWrongGuess means that the challenge is open and the code is wrong, so the guess counts toward the limit.
	CodeWrongGuess
	CodeAccepted
)

const maxWrongGuesses = 5

// Check applies the rules of a stored challenge to a code. It compares the code only when the challenge is open.
func (c Challenge) Check(key []byte, subject, email string, purpose CodePurpose, code string, now time.Time) CodeCheck {
	if !c.open(email, now) {
		return CodeRejected
	}
	return c.result(c.matches(key, subject, email, purpose, code))
}

// CheckInFixedTime applies the same rules as Check, but it always makes 1 keyed comparison of the code, also for a
// missing or closed challenge.
func (c Challenge) CheckInFixedTime(key []byte, subject, email string, purpose CodePurpose, code string, now time.Time) CodeCheck {
	matched := c.matches(key, subject, email, purpose, code)
	if !c.open(email, now) {
		return CodeRejected
	}
	return c.result(matched)
}

func (Challenge) result(matched bool) CodeCheck {
	if !matched {
		return CodeWrongGuess
	}
	return CodeAccepted
}

func (c Challenge) open(email string, now time.Time) bool {
	return c.Email == email && c.WrongGuesses < maxWrongGuesses && now.Before(c.ExpiresAt)
}

func (c Challenge) matches(key []byte, subject, email string, purpose CodePurpose, code string) bool {
	return matchesVerifier(key, subject, email, purpose, code, c.Verifier)
}

func VerifyChallenge(key []byte, subject, address string, purpose CodePurpose, code string, expected [32]byte, expiresAt, now time.Time) bool {
	return now.Before(expiresAt) && matchesVerifier(key, subject, address, purpose, code, expected)
}

func matchesVerifier(key []byte, subject, address string, purpose CodePurpose, code string, expected [32]byte) bool {
	if len(key) != 32 || subject == "" || !validPurpose(purpose) || len(code) != 6 {
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
	return purpose == PurposeVerifyEmail || purpose == PurposeClaimAccount || purpose == PurposePasswordReset || purpose == PurposeReauthenticate
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
