package email

import (
	"context"
	"errors"
	"testing"

	"github.com/vasapolrittideah/flowspace-api/services/identity/internal/domain"
)

func TestMailpitSenderRejectsUnsafeInputs(t *testing.T) {
	if _, err := NewMailpitSender("invalid", "no-reply@flowspace.local"); !errors.Is(err, ErrMailDelivery) {
		t.Fatalf("invalid SMTP address = %v", err)
	}
	if _, err := NewMailpitSender("127.0.0.1:1", "not an email"); !errors.Is(err, ErrMailDelivery) {
		t.Fatalf("invalid sender address = %v", err)
	}
	sender, err := NewMailpitSender("127.0.0.1:1", "no-reply@flowspace.local")
	if err != nil {
		t.Fatal(err)
	}
	for _, input := range []struct{ recipient, code, purpose string }{
		{"Recipient@example.com\r\nBcc: attacker@example.com", "123456", "verify-email"},
		{"Recipient@example.com", "１２３４５６", "verify-email"},
		{"Recipient@example.com", "123456", "unknown"},
	} {
		if err := sender.Send(context.Background(), input.recipient, input.code, input.purpose); !errors.Is(err, ErrMailDelivery) {
			t.Fatalf("unsafe mail input = %v", err)
		}
	}
}

func TestPasswordResetMessage(t *testing.T) {
	address, subject, err := validateMessage("Recipient@example.com", "123456", string(domain.PurposePasswordReset))
	if err != nil || address != "Recipient@example.com" || subject != "FlowSpace password reset code" {
		t.Fatalf("recovery message: address=%q subject=%q error=%v", address, subject, err)
	}
}
