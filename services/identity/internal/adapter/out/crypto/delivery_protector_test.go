package crypto_test

import (
	"bytes"
	"strings"
	"testing"

	deliverycrypto "github.com/vasapolrittideah/flowspace-api/services/identity/internal/adapter/out/crypto"
)

func TestDeliveryProtector(t *testing.T) {
	protector, err := deliverycrypto.NewDeliveryProtector(bytes.Repeat([]byte{1}, 32), 1)
	if err != nil {
		t.Fatal(err)
	}
	material, err := protector.Protect("challenge", "verify-email", "subject", "User@example.com", "123456")
	if err != nil || strings.Contains(string(material.Ciphertext), "123456") || strings.Contains(string(material.Ciphertext), "User@example.com") {
		t.Fatalf("unprotected delivery material: %v", err)
	}
	email, code, err := protector.Open("challenge", "verify-email", "subject", material)
	if err != nil || email != "User@example.com" || code != "123456" {
		t.Fatalf("opened %q, %q, %v", email, code, err)
	}
	if _, _, err := protector.Open("other", "verify-email", "subject", material); err == nil {
		t.Fatal("changed challenge ID was accepted")
	}
	wrongVersion := material
	wrongVersion.KeyVersion++
	if _, _, err := protector.Open("challenge", "verify-email", "subject", wrongVersion); err == nil {
		t.Fatal("unknown key version was accepted")
	}
	broken := material
	broken.Ciphertext = bytes.Clone(material.Ciphertext)
	broken.Ciphertext[0] ^= 1
	if _, _, err := protector.Open("challenge", "verify-email", "subject", broken); err == nil {
		t.Fatal("changed ciphertext was accepted")
	}
	if _, err := protector.Protect("", "verify-email", "subject", "User@example.com", "123456"); err == nil {
		t.Fatal("missing challenge ID was accepted")
	}
	if _, err := deliverycrypto.NewDeliveryProtector(make([]byte, 31), 1); err == nil {
		t.Fatal("short encryption key was accepted")
	}
}
