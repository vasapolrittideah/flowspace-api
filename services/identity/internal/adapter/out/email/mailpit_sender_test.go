package email_test

import (
	"context"
	"errors"
	"testing"

	identityemail "github.com/vasapolrittideah/flowspace-api/services/identity/internal/adapter/out/email"
)

func TestMailpitSenderRejectsUnsafeInputs(t *testing.T) {
	if _, err := identityemail.NewMailpitSender("invalid", "no-reply@flowspace.local"); !errors.Is(err, identityemail.ErrMailDelivery) {
		t.Fatalf("invalid SMTP address = %v", err)
	}
	if _, err := identityemail.NewMailpitSender("127.0.0.1:1", "not an email"); !errors.Is(err, identityemail.ErrMailDelivery) {
		t.Fatalf("invalid sender address = %v", err)
	}
	sender, err := identityemail.NewMailpitSender("127.0.0.1:1", "no-reply@flowspace.local")
	if err != nil {
		t.Fatal(err)
	}
	for _, input := range []struct{ recipient, code, purpose string }{
		{"Recipient@example.com\r\nBcc: attacker@example.com", "123456", "verify-email"},
		{"Recipient@example.com", "１２３４５６", "verify-email"},
		{"Recipient@example.com", "123456", "unknown"},
	} {
		if err := sender.Send(context.Background(), input.recipient, input.code, input.purpose); !errors.Is(err, identityemail.ErrMailDelivery) {
			t.Fatalf("unsafe mail input = %v", err)
		}
	}
}
