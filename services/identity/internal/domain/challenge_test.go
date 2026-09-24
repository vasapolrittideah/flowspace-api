package domain

import (
	"bytes"
	"strings"
	"testing"
	"time"
)

func TestNewCodePreservesLeadingZeroes(t *testing.T) {
	code, err := newCode(bytes.NewReader([]byte{0, 0, 0}))
	if err != nil || code != "000000" {
		t.Fatalf("newCode() = %q, %v", code, err)
	}
}

func TestChallengeVerifierBindsIdentityPurposeAndExpiry(t *testing.T) {
	key := bytes.Repeat([]byte{1}, 32)
	issuedAt := time.Date(2026, 9, 25, 0, 0, 0, 0, time.UTC)
	code, verifier, expiresAt, err := NewChallenge(key, "subject-1", "User@EXAMPLE.com", PurposeVerifyEmail, issuedAt)
	if err != nil {
		t.Fatal(err)
	}
	if len(code) != 6 || expiresAt != issuedAt.Add(10*time.Minute) {
		t.Fatalf("code length = %d, expiry = %s", len(code), expiresAt)
	}
	for _, digit := range code {
		if digit < '0' || digit > '9' {
			t.Fatalf("code contains non-ASCII digit: %q", code)
		}
	}
	if !VerifyChallenge(key, "subject-1", "User@example.com", PurposeVerifyEmail, code, verifier, expiresAt, issuedAt.Add(10*time.Minute-time.Nanosecond)) {
		t.Fatal("valid code was rejected before expiry")
	}
	for _, test := range []struct {
		name    string
		key     []byte
		subject string
		email   string
		purpose CodePurpose
		code    string
		now     time.Time
	}{
		{"expiry", key, "subject-1", "User@example.com", PurposeVerifyEmail, code, expiresAt},
		{"other subject", key, "subject-2", "User@example.com", PurposeVerifyEmail, code, issuedAt},
		{"other local part", key, "subject-1", "user@example.com", PurposeVerifyEmail, code, issuedAt},
		{"other purpose", key, "subject-1", "User@example.com", PurposeClaimAccount, code, issuedAt},
		{"other key", bytes.Repeat([]byte{2}, 32), "subject-1", "User@example.com", PurposeVerifyEmail, code, issuedAt},
		{"malformed code", key, "subject-1", "User@example.com", PurposeVerifyEmail, "１２３４５６", issuedAt},
	} {
		if VerifyChallenge(test.key, test.subject, test.email, test.purpose, test.code, verifier, expiresAt, test.now) {
			t.Errorf("accepted %s", test.name)
		}
	}
}

func TestNewChallengeRejectsInvalidInputsWithoutSecrets(t *testing.T) {
	key := bytes.Repeat([]byte{1}, 32)
	for _, test := range []struct {
		key     []byte
		subject string
		email   string
		purpose CodePurpose
	}{
		{key[:31], "subject", "user@example.com", PurposeVerifyEmail},
		{key, "", "user@example.com", PurposeVerifyEmail},
		{key, "subject", "secret-invalid-address", PurposeVerifyEmail},
		{key, "subject", "user@example.com", "unknown"},
	} {
		_, _, _, err := NewChallenge(test.key, test.subject, test.email, test.purpose, time.Now())
		if err == nil || strings.Contains(err.Error(), "secret") {
			t.Errorf("invalid challenge input returned %v", err)
		}
	}
}
