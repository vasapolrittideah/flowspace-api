package domain

import "testing"

func TestNormalizeEmail(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"User.Name@EXAMPLE.COM", "User.Name@example.com"},
		{"user.name@example.com", "user.name@example.com"},
		{"username@example.com", "username@example.com"},
		{"User@example.com", "User@example.com"},
		{"user@example.com", "user@example.com"},
	}
	for _, test := range tests {
		got, err := NormalizeEmail(test.input)
		if err != nil || got != test.want {
			t.Errorf("NormalizeEmail(%q) = %q, %v; want %q", test.input, got, err, test.want)
		}
	}
}

func TestNormalizeEmailRejectsInvalidInput(t *testing.T) {
	for _, input := range []string{
		"", " user@example.com", "user@example.com ", "Name <user@example.com>",
		"üser@example.com", "user@exämple.com", "user@example.com\r\nBcc:other@example.com", "user@@example.com",
	} {
		if _, err := NormalizeEmail(input); err == nil {
			t.Errorf("NormalizeEmail(%q) accepted invalid address", input)
		}
	}
}
