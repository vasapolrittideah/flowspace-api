package outbound

type DeliveryMaterial struct {
	KeyVersion int32
	Nonce      []byte
	Ciphertext []byte
}
