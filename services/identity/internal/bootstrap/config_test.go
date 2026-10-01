package bootstrap

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"go.uber.org/zap"

	sharedconfig "github.com/vasapolrittideah/flowspace-api/internal/config"
	"github.com/vasapolrittideah/flowspace-api/services/identity/internal/app"
	"github.com/vasapolrittideah/flowspace-api/services/identity/internal/domain"
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

func TestLoadAPIConfigRequiresCompleteSessionListener(t *testing.T) {
	t.Setenv("ENVIRONMENT", "local")
	t.Setenv("DATABASE_URL", "postgres://identity:secret@localhost/identity")
	t.Setenv("SIGNING_KEY_FILE", "/keys/signing.pem")
	t.Setenv("SIGNING_KEY_ID", "local-1")
	t.Setenv("TOKEN_ISSUER", "urn:flowspace:identity:local")
	t.Setenv("TOKEN_AUDIENCE", "flowspace-api")
	t.Setenv("CODE_VERIFIER_KEY_FILE", "/keys/verifier")
	t.Setenv("DELIVERY_KEY_FILE", "/keys/delivery")
	t.Setenv("SESSION_GRPC_ADDR", ":8082")
	if _, err := LoadAPIConfig(); err == nil || strings.Contains(err.Error(), "secret") {
		t.Fatalf("accepted or disclosed incomplete session configuration: %v", err)
	}
	t.Setenv("SESSION_TLS_CERT_FILE", "/keys/session/server.pem")
	t.Setenv("SESSION_TLS_KEY_FILE", "/keys/session/server-key.pem")
	t.Setenv("SESSION_CLIENT_CA_FILE", "/keys/session/ca.pem")
	t.Setenv("SESSION_CALLER_ALLOWLIST", "urn:flowspace:service:workspace=aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	if _, err := LoadAPIConfig(); err != nil {
		t.Fatalf("complete session configuration: %v", err)
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

const mountedGoogleFile = "/keys/google/client-secret"

func TestProviderClientsUseExactCallbackURLs(t *testing.T) {
	clients, err := providerClients(APIConfig{
		Environment: "production", GoogleClientID: "google-client", GoogleClientSecretFile: mountedGoogleFile,
		GitHubClientID: "github-client", ProviderCallbackBaseURL: "https://api.example.com/",
	})
	if err != nil {
		t.Fatal(err)
	}
	want := map[domain.Provider]app.ProviderClient{
		domain.ProviderGoogle: {ClientID: "google-client", CallbackURL: "https://api.example.com/v1/provider-login-callbacks/google"},
		domain.ProviderGitHub: {ClientID: "github-client", CallbackURL: "https://api.example.com/v1/provider-login-callbacks/github"},
	}
	if !maps.Equal(clients, want) {
		t.Fatalf("clients = %v", clients)
	}

	clients, err = providerClients(APIConfig{Environment: "local", GitHubClientID: "github-client", ProviderCallbackBaseURL: "http://localhost:8080"})
	if err != nil || len(clients) != 1 || clients[domain.ProviderGitHub].CallbackURL != "http://localhost:8080/v1/provider-login-callbacks/github" {
		t.Fatalf("local GitHub client = %v, %v", clients, err)
	}
	if clients, err := providerClients(APIConfig{Environment: "local"}); err != nil || len(clients) != 0 {
		t.Fatalf("unconfigured providers = %v, %v", clients, err)
	}
}

func TestProviderClientsRejectUnsafeCallbackBase(t *testing.T) {
	for _, test := range []struct {
		name   string
		config APIConfig
	}{
		{"missing base", APIConfig{Environment: "local", GoogleClientID: "google-client", GoogleClientSecretFile: mountedGoogleFile}},
		{"Google client without secret file", APIConfig{Environment: "local", GoogleClientID: "google-client", ProviderCallbackBaseURL: "https://api.example.com"}},
		{"base without client", APIConfig{Environment: "local", ProviderCallbackBaseURL: "https://api.example.com"}},
		{"plain HTTP outside local", APIConfig{Environment: "production", GoogleClientID: "google-client", GoogleClientSecretFile: mountedGoogleFile, ProviderCallbackBaseURL: "http://api.example.com"}},
		{"relative", APIConfig{Environment: "local", GoogleClientID: "google-client", GoogleClientSecretFile: mountedGoogleFile, ProviderCallbackBaseURL: "/callbacks"}},
		{"path", APIConfig{Environment: "local", GoogleClientID: "google-client", GoogleClientSecretFile: mountedGoogleFile, ProviderCallbackBaseURL: "https://api.example.com/identity"}},
		{"query", APIConfig{Environment: "local", GoogleClientID: "google-client", GoogleClientSecretFile: mountedGoogleFile, ProviderCallbackBaseURL: "https://api.example.com?next=https://attacker.example"}},
		{"fragment", APIConfig{Environment: "local", GoogleClientID: "google-client", GoogleClientSecretFile: mountedGoogleFile, ProviderCallbackBaseURL: "https://api.example.com#next"}},
		{"user info", APIConfig{Environment: "local", GoogleClientID: "google-client", GoogleClientSecretFile: mountedGoogleFile, ProviderCallbackBaseURL: "https://user@api.example.com"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			if clients, err := providerClients(test.config); err == nil {
				t.Fatalf("accepted %v", clients)
			}
		})
	}
}

func TestLoadAPIConfigLoadsProviderClients(t *testing.T) {
	t.Setenv("ENVIRONMENT", "local")
	t.Setenv("DATABASE_URL", "postgres://identity:secret@localhost/identity")
	t.Setenv("SIGNING_KEY_FILE", "/keys/signing.pem")
	t.Setenv("SIGNING_KEY_ID", "local-1")
	t.Setenv("TOKEN_ISSUER", "urn:flowspace:identity:local")
	t.Setenv("TOKEN_AUDIENCE", "flowspace-api")
	t.Setenv("CODE_VERIFIER_KEY_FILE", "/keys/verifier")
	t.Setenv("DELIVERY_KEY_FILE", "/keys/delivery")
	secretFile := filepath.Join(t.TempDir(), "client-secret")
	if err := os.WriteFile(secretFile, []byte("google-secret-value\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GOOGLE_CLIENT_ID", "google-client")
	t.Setenv("GOOGLE_CLIENT_SECRET_FILE", secretFile)
	t.Setenv("PROVIDER_CALLBACK_BASE_URL", "http://localhost:8080")
	config, err := LoadAPIConfig()
	if err != nil || config.providers[domain.ProviderGoogle].CallbackURL != "http://localhost:8080/v1/provider-login-callbacks/google" ||
		config.GoogleClientSecret != "google-secret-value" {
		t.Fatalf("providers = %v, %v", config.providers, err)
	}
	if formatted := fmt.Sprintf("%v %+v", config, config); strings.Contains(formatted, "google-secret-value") {
		t.Fatal("formatted configuration contains the Google client secret")
	}
	t.Setenv("PROVIDER_CALLBACK_BASE_URL", "")
	if _, err := LoadAPIConfig(); err == nil {
		t.Fatal("accepted a provider without a callback base")
	}
	t.Setenv("PROVIDER_CALLBACK_BASE_URL", "http://localhost:8080")
	for _, invalid := range []string{filepath.Join(t.TempDir(), "missing"), writeFile(t, "  \n")} {
		t.Setenv("GOOGLE_CLIENT_SECRET_FILE", invalid)
		if _, err := LoadAPIConfig(); err == nil {
			t.Fatalf("accepted Google client secret file %q", invalid)
		}
	}
}

func writeFile(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}
