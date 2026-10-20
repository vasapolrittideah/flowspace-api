package postgres

import (
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/vasapolrittideah/flowspace-api/services/identity/internal/adapter/out/postgres/sqlc"
)

func TestReauthenticationDeliveryRequiresVerifiedCurrentEmail(t *testing.T) {
	account := sqlc.GetDeliveryAccountForUpdateRow{EmailLocal: "User", EmailDomain: "example.com", EmailVerifiedAt: pgtype.Timestamptz{Valid: true}}
	challenge := sqlc.GetDeliveryChallengeForUpdateRow{Purpose: "reauthenticate", EmailLocal: "User", EmailDomain: "example.com"}
	if !isCurrentDelivery(account, challenge) {
		t.Fatal("verified account re-authentication delivery rejected")
	}
	account.EmailVerifiedAt.Valid = false
	if isCurrentDelivery(account, challenge) {
		t.Fatal("sent re-authentication code to unverified account")
	}
	account.EmailVerifiedAt.Valid = true
	challenge.EmailLocal = "Previous"
	if isCurrentDelivery(account, challenge) {
		t.Fatal("sent re-authentication code to a previous email")
	}
}
