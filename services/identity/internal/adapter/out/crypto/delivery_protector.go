package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/json"
	"errors"

	outbound "github.com/vasapolrittideah/flowspace-api/services/identity/internal/port/out"
)

var ErrDeliveryProtection = errors.New("delivery protection failed")

type DeliveryProtector struct {
	aead    cipher.AEAD
	version int32
}

var _ outbound.DeliveryProtector = (*DeliveryProtector)(nil)

func NewDeliveryProtector(key []byte, version int32) (*DeliveryProtector, error) {
	if len(key) != 32 || version < 1 {
		return nil, ErrDeliveryProtection
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, ErrDeliveryProtection
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, ErrDeliveryProtection
	}
	return &DeliveryProtector{aead: aead, version: version}, nil
}

func (p *DeliveryProtector) Protect(challengeID, purpose, subject, email, code string) (outbound.DeliveryMaterial, error) {
	if challengeID == "" || purpose == "" || subject == "" || email == "" || code == "" {
		return outbound.DeliveryMaterial{}, ErrDeliveryProtection
	}
	plain, err := json.Marshal(struct {
		Email string `json:"email"`
		Code  string `json:"code"`
	}{email, code})
	if err != nil {
		return outbound.DeliveryMaterial{}, ErrDeliveryProtection
	}
	nonce := make([]byte, p.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return outbound.DeliveryMaterial{}, ErrDeliveryProtection
	}
	associated, err := json.Marshal([3]string{challengeID, purpose, subject})
	if err != nil {
		return outbound.DeliveryMaterial{}, ErrDeliveryProtection
	}
	return outbound.DeliveryMaterial{
		KeyVersion: p.version, Nonce: nonce, Ciphertext: p.aead.Seal(nil, nonce, plain, associated),
	}, nil
}

func (p *DeliveryProtector) Open(challengeID, purpose, subject string, material outbound.DeliveryMaterial) (string, string, error) {
	if material.KeyVersion != p.version || len(material.Nonce) != p.aead.NonceSize() {
		return "", "", ErrDeliveryProtection
	}
	associated, err := json.Marshal([3]string{challengeID, purpose, subject})
	if err != nil {
		return "", "", ErrDeliveryProtection
	}
	plain, err := p.aead.Open(nil, material.Nonce, material.Ciphertext, associated)
	if err != nil {
		return "", "", ErrDeliveryProtection
	}
	var payload struct {
		Email string `json:"email"`
		Code  string `json:"code"`
	}
	if err := json.Unmarshal(plain, &payload); err != nil || payload.Email == "" || payload.Code == "" {
		return "", "", ErrDeliveryProtection
	}
	return payload.Email, payload.Code, nil
}
