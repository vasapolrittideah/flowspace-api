package bootstrap

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"go.uber.org/zap"

	sharedconfig "github.com/vasapolrittideah/flowspace-api/internal/config"
)

func TestLoadAPIConfigRequiresKeysWithoutLeakingValues(t *testing.T) {
	t.Setenv("ENVIRONMENT", "local")
	t.Setenv("DATABASE_URL", "postgres://identity:secret@localhost/identity")
	t.Setenv("SIGNING_KEY_FILE", "")
	t.Setenv("CODE_VERIFIER_KEY_FILE", "")
	t.Setenv("DELIVERY_KEY_FILE", "")
	_, err := LoadAPIConfig()
	if err == nil || strings.Contains(err.Error(), "secret") {
		t.Fatalf("expected safe missing-key error, got %v", err)
	}
}

func TestLoadAPIAndWorkerConfig(t *testing.T) {
	t.Setenv("ENVIRONMENT", "local")
	t.Setenv("DATABASE_URL", "postgres://identity:secret@localhost/identity")
	t.Setenv("SIGNING_KEY_FILE", "/keys/signing.pem")
	t.Setenv("SIGNING_KEY_ID", "local-1")
	t.Setenv("TOKEN_ISSUER", "urn:flowspace:identity:local")
	t.Setenv("TOKEN_AUDIENCE", "flowspace-api")
	t.Setenv("CODE_VERIFIER_KEY_FILE", "/keys/verifier")
	t.Setenv("DELIVERY_KEY_FILE", "/keys/delivery")
	api, err := LoadAPIConfig()
	if err != nil || api.OutboxReadyMaxPending != 10000 || api.HTTPAddress != ":8080" {
		t.Fatalf("API config = %+v, %v", api, err)
	}
	t.Setenv("OUTBOX_READY_MAX_PENDING", "0")
	if _, err := LoadAPIConfig(); err == nil {
		t.Fatal("accepted zero outbox capacity")
	}
	t.Setenv("OUTBOX_READY_MAX_PENDING", "10000")
	t.Setenv("TOKEN_ISSUER", "wrong")
	if _, err := LoadAPIConfig(); err == nil {
		t.Fatal("accepted wrong local issuer")
	}
	t.Setenv("BROKER_ADDR", "localhost:9092")
	t.Setenv("SCHEMA_REGISTRY_URL", "http://localhost:8081")
	t.Setenv("DELIVERY_TOPIC", "identity-email")
	t.Setenv("DELIVERY_GROUP", "identity-worker")
	t.Setenv("MAIL_ADDR", "localhost:1025")
	t.Setenv("MAIL_FROM", "codes@example.com")
	t.Setenv("BROKER_USERNAME", "identity-worker")
	t.Setenv("BROKER_PASSWORD", "local-secret")
	worker, err := LoadWorkerConfig()
	if err != nil || worker.BrokerAddress != "localhost:9092" || worker.HealthAddress != ":8081" || worker.BrokerUsername != "identity-worker" {
		t.Fatalf("worker config = %+v, %v", worker, err)
	}
	t.Setenv("BROKER_PASSWORD", "")
	if _, err := LoadWorkerConfig(); err == nil || strings.Contains(err.Error(), "local-secret") {
		t.Fatalf("accepted missing broker password or leaked its value: %v", err)
	}
	t.Setenv("BROKER_PASSWORD", "local-secret")
	t.Setenv("BROKER_ADDR", "")
	if _, err := LoadWorkerConfig(); err == nil {
		t.Fatal("accepted missing broker address")
	}
}

func TestReadKeyRejectsWrongLengthWithoutPrintingContents(t *testing.T) {
	path := filepath.Join(t.TempDir(), "key")
	if err := os.WriteFile(path, []byte("sensitive-key-material"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := readKey(path)
	if err == nil || strings.Contains(err.Error(), "sensitive-key-material") {
		t.Fatalf("expected safe invalid-key error, got %v", err)
	}
}

func TestRuntimeRejectsInvalidKeysAndConfiguration(t *testing.T) {
	t.Setenv("FLOWSPACE_TEST_INVALID_DSN", "invalid-dsn")
	t.Setenv("FLOWSPACE_TEST_SIGNING_KEY_ID", "local-1")
	t.Setenv("FLOWSPACE_TEST_ISSUER", "urn:flowspace:identity:local")
	t.Setenv("FLOWSPACE_TEST_AUDIENCE", "flowspace-api")
	directory := t.TempDir()
	_, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	der, err := x509.MarshalPKCS8PrivateKey(privateKey)
	if err != nil {
		t.Fatal(err)
	}
	signing := filepath.Join(directory, "signing.pem")
	verifier := filepath.Join(directory, "verifier")
	delivery := filepath.Join(directory, "delivery")
	for path, value := range map[string][]byte{
		signing:  pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der}),
		verifier: make([]byte, 32), delivery: make([]byte, 32),
	} {
		if err := os.WriteFile(path, value, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	api := APIConfig{
		SigningKeyFile: signing, CodeVerifierKeyFile: verifier, DeliveryKeyFile: delivery,
		SigningKeyID: os.Getenv("FLOWSPACE_TEST_SIGNING_KEY_ID"), TokenIssuer: os.Getenv("FLOWSPACE_TEST_ISSUER"), TokenAudience: os.Getenv("FLOWSPACE_TEST_AUDIENCE"),
		DatabaseURL: sharedconfig.Secret(os.Getenv("FLOWSPACE_TEST_INVALID_DSN")), OutboxReadyMaxPending: 10000,
	}
	for _, field := range []string{"signing", "verifier", "delivery", "proxy", "signer", "database"} {
		candidate := api
		switch field {
		case "signing":
			candidate.SigningKeyFile = filepath.Join(directory, "missing")
		case "verifier":
			candidate.CodeVerifierKeyFile = filepath.Join(directory, "missing")
		case "delivery":
			candidate.DeliveryKeyFile = filepath.Join(directory, "missing")
		case "proxy":
			candidate.TrustedProxyCIDRs = "0.0.0.0/0"
		case "signer":
			candidate.SigningKeyID = ""
		}
		if _, err := NewAPIServer(context.Background(), candidate, zap.NewNop()); err == nil || strings.Contains(err.Error(), "invalid-dsn") {
			t.Fatalf("API accepted or disclosed %s configuration: %v", field, err)
		}
	}
}

func TestWorkerRejectsInvalidConfiguration(t *testing.T) {
	t.Setenv("FLOWSPACE_TEST_INVALID_DSN", "invalid-dsn")
	directory := t.TempDir()
	delivery := filepath.Join(directory, "delivery")
	if err := os.WriteFile(delivery, make([]byte, 32), 0o600); err != nil {
		t.Fatal(err)
	}
	worker := WorkerConfig{
		DeliveryKeyFile: delivery, MailAddress: "127.0.0.1:1025", MailFrom: "codes@example.com",
		DatabaseURL: sharedconfig.Secret(os.Getenv("FLOWSPACE_TEST_INVALID_DSN")),
	}
	for _, field := range []string{"key", "mail", "database"} {
		candidate := worker
		switch field {
		case "key":
			candidate.DeliveryKeyFile = filepath.Join(directory, "missing")
		case "mail":
			candidate.MailAddress = "invalid"
		}
		if _, err := NewWorker(context.Background(), candidate, zap.NewNop()); err == nil || strings.Contains(err.Error(), "invalid-dsn") {
			t.Fatalf("worker accepted or disclosed %s configuration: %v", field, err)
		}
	}
}
