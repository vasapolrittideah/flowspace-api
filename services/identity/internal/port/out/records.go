package outbound

import "time"

type AccountState struct {
	Email         string
	EmailVerified bool
}

type ChallengeState struct {
	ID           string
	Email        string
	Verifier     [32]byte
	WrongGuesses int16
	ExpiresAt    time.Time
}

type ClaimAccount struct {
	Subject       string
	EmailVerified bool
}

type DeliveryMaterial struct {
	KeyVersion int32
	Nonce      []byte
	Ciphertext []byte
}
