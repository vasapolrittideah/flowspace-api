package domain

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"strings"
	"unicode/utf8"

	"golang.org/x/crypto/argon2"
	"golang.org/x/text/unicode/norm"
)

var commonPasswordsV1 = map[string]struct{}{
	"password123": {}, "qwerty123": {}, "flowspace123": {},
}

func HashPassword(ctx context.Context, password string, compromised func(context.Context, string) (bool, error)) (string, error) {
	password, err := ValidatePassword(password)
	if err != nil {
		return "", err
	}
	if compromised == nil {
		return "", ErrPasswordCheckUnavailable
	}
	blocked, err := compromised(ctx, password)
	if err != nil {
		return "", ErrPasswordCheckUnavailable
	}
	if blocked {
		return "", ErrInvalidPassword
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return "", ErrPasswordHashingUnavailable
	}
	hash := argon2.IDKey([]byte(password), salt, 2, 19456, 1, 32)
	return fmt.Sprintf("$argon2id$v=%d$m=19456,t=2,p=1$%s$%s", argon2.Version,
		base64.RawStdEncoding.EncodeToString(salt), base64.RawStdEncoding.EncodeToString(hash)), nil
}

func VerifyPassword(ctx context.Context, password, stored string) (bool, error) {
	password, err := NormalizeLoginPassword(password)
	if err != nil {
		return false, err
	}
	parts := strings.Split(stored, "$")
	if len(parts) != 6 || parts[0] != "" || parts[1] != "argon2id" ||
		parts[2] != fmt.Sprintf("v=%d", argon2.Version) || parts[3] != "m=19456,t=2,p=1" {
		return false, ErrPasswordHashingUnavailable
	}
	salt, saltErr := base64.RawStdEncoding.DecodeString(parts[4])
	expected, hashErr := base64.RawStdEncoding.DecodeString(parts[5])
	if saltErr != nil || hashErr != nil || len(salt) != 16 || len(expected) != 32 {
		return false, ErrPasswordHashingUnavailable
	}
	if err := ctx.Err(); err != nil {
		return false, err
	}
	actual := argon2.IDKey([]byte(password), salt, 2, 19456, 1, 32)
	if err := ctx.Err(); err != nil {
		return false, err
	}
	return subtle.ConstantTimeCompare(actual, expected) == 1, nil
}

func NormalizeLoginPassword(password string) (string, error) {
	if !utf8.ValidString(password) || password == "" {
		return "", ErrInvalidPassword
	}
	password = norm.NFC.String(password)
	if utf8.RuneCountInString(password) > 64 {
		return "", ErrInvalidPassword
	}
	return password, nil
}

func ValidatePassword(password string) (string, error) {
	if !utf8.ValidString(password) {
		return "", ErrInvalidPassword
	}
	password = norm.NFC.String(password)
	length := utf8.RuneCountInString(password)
	if length < 8 || length > 64 {
		return "", ErrInvalidPassword
	}
	if length < 15 {
		lower, digit := false, false
		for i := range len(password) {
			lower = lower || password[i] >= 'a' && password[i] <= 'z'
			digit = digit || password[i] >= '0' && password[i] <= '9'
		}
		if !lower || !digit {
			return "", ErrInvalidPassword
		}
	}
	if _, blocked := commonPasswordsV1[strings.ToLower(password)]; blocked {
		return "", ErrInvalidPassword
	}
	return password, nil
}
