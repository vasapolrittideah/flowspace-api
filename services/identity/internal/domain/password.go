package domain

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"golang.org/x/crypto/argon2"
	"golang.org/x/text/unicode/norm"
)

var (
	ErrInvalidPassword          = errors.New("invalid password")
	ErrPasswordCheckUnavailable = errors.New("password check unavailable")
	commonPasswordsV1           = map[string]struct{}{
		"password123": {}, "qwerty123": {}, "flowspace123": {},
	}
)

func HashPassword(ctx context.Context, password string, compromised func(context.Context, string) (bool, error)) (string, error) {
	password, err := validatePassword(password)
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
		return "", errors.New("password hashing unavailable")
	}
	hash := argon2.IDKey([]byte(password), salt, 2, 19456, 1, 32)
	return fmt.Sprintf("$argon2id$v=%d$m=19456,t=2,p=1$%s$%s", argon2.Version,
		base64.RawStdEncoding.EncodeToString(salt), base64.RawStdEncoding.EncodeToString(hash)), nil
}

func validatePassword(password string) (string, error) {
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
