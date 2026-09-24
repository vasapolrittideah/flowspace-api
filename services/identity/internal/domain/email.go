package domain

import (
	"net/mail"
	"strings"
)

func NormalizeEmail(address string) (string, error) {
	for i := range len(address) {
		if address[i] > 127 {
			return "", ErrInvalidEmail
		}
	}
	parsed, err := mail.ParseAddress(address)
	if err != nil || parsed.Name != "" || parsed.Address != address {
		return "", ErrInvalidEmail
	}
	at := strings.LastIndexByte(address, '@')
	if at < 1 || at == len(address)-1 {
		return "", ErrInvalidEmail
	}
	return address[:at+1] + strings.ToLower(address[at+1:]), nil
}
