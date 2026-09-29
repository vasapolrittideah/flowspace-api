package email

import (
	"testing"

	"github.com/vasapolrittideah/flowspace-api/services/identity/internal/domain"
)

func TestPasswordResetMessage(t *testing.T) {
	address, subject, err := validateMessage("Recipient@example.com", "123456", string(domain.PurposePasswordReset))
	if err != nil || address != "Recipient@example.com" || subject != "FlowSpace password reset code" {
		t.Fatalf("recovery message: address=%q subject=%q error=%v", address, subject, err)
	}
}
