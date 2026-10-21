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

func TestPasswordResetChallengeCannotCrossPurposes(t *testing.T) {
	key := bytes.Repeat([]byte{1}, 32)
	now := time.Now()
	code, verifier, expiresAt, err := NewChallenge(key, "subject", "User@example.com", PurposePasswordReset, now)
	if err != nil {
		t.Fatal(err)
	}
	if !VerifyChallenge(key, "subject", "User@example.com", PurposePasswordReset, code, verifier, expiresAt, now) {
		t.Fatal("reset challenge rejected")
	}
	for _, purpose := range []CodePurpose{PurposeVerifyEmail, PurposeClaimAccount} {
		if VerifyChallenge(key, "subject", "User@example.com", purpose, code, verifier, expiresAt, now) {
			t.Fatalf("reset challenge accepted for %s", purpose)
		}
	}
}

func TestReauthenticationChallengePurposeAndExpiry(t *testing.T) {
	key := bytes.Repeat([]byte{7}, 32)
	now := time.Date(2026, 10, 20, 0, 0, 0, 0, time.UTC)
	purpose := CodePurpose("reauthenticate")
	code, verifier, expiry, err := NewChallenge(key, "subject", "User@example.com", purpose, now)
	if err != nil {
		t.Fatalf("re-authentication challenge rejected: %v", err)
	}
	if !VerifyChallenge(key, "subject", "User@example.com", purpose, code, verifier, expiry, expiry.Add(-time.Nanosecond)) {
		t.Fatal("valid re-authentication code rejected")
	}
	if VerifyChallenge(key, "subject", "User@example.com", purpose, code, verifier, expiry, expiry) {
		t.Fatal("code accepted at exact expiry")
	}
	for _, other := range []CodePurpose{PurposeVerifyEmail, PurposeClaimAccount, PurposePasswordReset} {
		if VerifyChallenge(key, "subject", "User@example.com", other, code, verifier, expiry, now) {
			t.Fatalf("re-authentication code accepted as %s", other)
		}
		otherCode, otherVerifier, otherExpiry, err := NewChallenge(key, "subject", "User@example.com", other, now)
		if err != nil {
			t.Fatal(err)
		}
		if VerifyChallenge(key, "subject", "User@example.com", purpose, otherCode, otherVerifier, otherExpiry, now) {
			t.Fatalf("%s code accepted for re-authentication", other)
		}
	}
}

func TestChallengeCheckAppliesStoredChallengeRules(t *testing.T) {
	key := bytes.Repeat([]byte{3}, 32)
	now := time.Date(2026, 10, 21, 0, 0, 0, 0, time.UTC)
	code, verifier, expiresAt, err := NewChallenge(key, "subject", "User@example.com", PurposeVerifyEmail, now)
	if err != nil {
		t.Fatal(err)
	}
	wrongCode := "000000"
	if code == wrongCode {
		wrongCode = "000001"
	}
	stored := Challenge{ID: "challenge", Email: "User@example.com", Verifier: verifier, WrongGuesses: 4, ExpiresAt: expiresAt}
	for _, test := range []struct {
		name      string
		challenge Challenge
		email     string
		code      string
		now       time.Time
		want      CodeCheck
	}{
		{"correct code", stored, "User@example.com", code, now, CodeAccepted},
		{"wrong code", stored, "User@example.com", wrongCode, now, CodeWrongGuess},
		{"wrong email", stored, "Other@example.com", code, now, CodeRejected},
		{"other local part case", stored, "user@example.com", code, now, CodeRejected},
		{"wrong-guess limit", Challenge{Email: "User@example.com", Verifier: verifier, WrongGuesses: 5, ExpiresAt: expiresAt}, "User@example.com", code, now, CodeRejected},
		{"expiry", stored, "User@example.com", code, expiresAt, CodeRejected},
		{"wrong code after expiry", stored, "User@example.com", wrongCode, expiresAt, CodeRejected},
		{"just before expiry", stored, "User@example.com", code, expiresAt.Add(-time.Nanosecond), CodeAccepted},
		{"zero challenge", Challenge{}, "User@example.com", code, now, CodeRejected},
	} {
		if got := test.challenge.Check(key, "subject", test.email, PurposeVerifyEmail, test.code, test.now); got != test.want {
			t.Errorf("%s: Check() = %v, want %v", test.name, got, test.want)
		}
	}
}

func TestChallengeMatchesComparesCodeWithoutStoredState(t *testing.T) {
	key := bytes.Repeat([]byte{4}, 32)
	now := time.Date(2026, 10, 21, 0, 0, 0, 0, time.UTC)
	code, verifier, expiresAt, err := NewChallenge(key, "subject", "User@example.com", PurposePasswordReset, now)
	if err != nil {
		t.Fatal(err)
	}
	spent := Challenge{Email: "Other@example.com", Verifier: verifier, WrongGuesses: 5, ExpiresAt: expiresAt}
	if !spent.matches(key, "subject", "User@example.com", PurposePasswordReset, code) {
		t.Fatal("matching code rejected")
	}
	if spent.open("User@example.com", now) {
		t.Fatal("spent challenge is open")
	}
	if (Challenge{}).matches(key, "subject", "User@example.com", PurposePasswordReset, code) {
		t.Fatal("zero challenge matched a code")
	}
	if !(Challenge{Email: "User@example.com", WrongGuesses: 4, ExpiresAt: expiresAt}).open("User@example.com", now) {
		t.Fatal("current challenge is not open")
	}
}

func TestChallengeCheckInFixedTimeAlwaysComparesTheCode(t *testing.T) {
	key := bytes.Repeat([]byte{5}, 32)
	now := time.Date(2026, 10, 21, 0, 0, 0, 0, time.UTC)
	code, verifier, expiresAt, err := NewChallenge(key, "subject", "User@example.com", PurposePasswordReset, now)
	if err != nil {
		t.Fatal(err)
	}
	open := Challenge{Email: "User@example.com", Verifier: verifier, ExpiresAt: expiresAt}
	if got := open.CheckInFixedTime(key, "subject", "User@example.com", PurposePasswordReset, code, now); got != CodeAccepted {
		t.Fatalf("CheckInFixedTime() = %v, want accepted", got)
	}
	comparison := testing.AllocsPerRun(100, func() {
		open.matches(key, "subject", "User@example.com", PurposePasswordReset, code)
	})
	if comparison == 0 {
		t.Fatal("a keyed comparison made no allocations")
	}
	for _, test := range []struct {
		name      string
		challenge Challenge
		now       time.Time
	}{
		{"open", open, now},
		{"zero challenge", Challenge{}, now},
		{"expired", open, expiresAt},
		{"wrong-guess limit", Challenge{Email: "User@example.com", Verifier: verifier, WrongGuesses: 5, ExpiresAt: expiresAt}, now},
		{"other email", Challenge{Email: "Other@example.com", Verifier: verifier, ExpiresAt: expiresAt}, now},
	} {
		if got := test.challenge.CheckInFixedTime(key, "subject", "User@example.com", PurposePasswordReset, code, test.now); test.name != "open" && got != CodeRejected {
			t.Errorf("%s: CheckInFixedTime() = %v, want rejected", test.name, got)
		}
		allocs := testing.AllocsPerRun(100, func() {
			test.challenge.CheckInFixedTime(key, "subject", "User@example.com", PurposePasswordReset, code, test.now)
		})
		if allocs != comparison {
			t.Errorf("%s: %v allocations, want %v from 1 keyed comparison", test.name, allocs, comparison)
		}
	}
}
