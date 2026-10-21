package outbound

type AccountState struct {
	Email         string
	EmailVerified bool
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
