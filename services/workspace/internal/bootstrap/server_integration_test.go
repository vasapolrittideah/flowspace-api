//go:build integration

package bootstrap

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/testcontainers/testcontainers-go"
	postgrescontainer "github.com/testcontainers/testcontainers-go/modules/postgres"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"

	sharedconfig "github.com/vasapolrittideah/flowspace-api/internal/config"
)

const integrationIssuer = "urn:flowspace:identity:test"

func TestServerStartsAndShutsDownWithDependencies(t *testing.T) {
	ctx := t.Context()
	container, err := postgrescontainer.Run(
		ctx,
		"postgres:18-alpine",
		postgrescontainer.WithDatabase("workspace"),
		postgrescontainer.WithUsername("workspace"),
		postgrescontainer.WithPassword("workspace"),
		postgrescontainer.BasicWaitStrategies(),
	)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := testcontainers.TerminateContainer(container); err != nil {
			t.Error(err)
		}
	})

	databaseURL, err := container.ConnectionString(ctx, "sslmode=disable", "application_name=workspace-api")
	if err != nil {
		t.Fatal(err)
	}
	certFile, keyFile, caFile, _, _ := testIdentityTLSFiles(t)
	jwksURL := testJWKSURL(t)
	failedDatabaseURL, err := container.ConnectionString(ctx, "sslmode=disable", "application_name=workspace-api-failed")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := NewServer(ctx, Config{
		HTTPAddress:     "127.0.0.1:0",
		DatabaseURL:     sharedconfig.Secret(failedDatabaseURL),
		IdentityJWKSURL: jwksURL,
		IdentityIssuer:  integrationIssuer, IdentityAudience: "flowspace-api",
		IdentitySessionAddress: "identity.test:8082", IdentitySessionServerName: "identity.test",
		IdentityClientCertFile: certFile, IdentityClientKeyFile: keyFile, IdentityCAFile: caFile,
		OIDCDiscoveryURL: "http://keycloak/missing",
	}, zap.NewNop()); err == nil {
		t.Fatal("NewServer() accepted failed OIDC discovery")
	}
	if _, err := NewServer(ctx, Config{
		DatabaseURL: sharedconfig.Secret(failedDatabaseURL), IdentityJWKSURL: jwksURL,
		IdentityIssuer: integrationIssuer, IdentityAudience: "flowspace-api",
		IdentitySessionAddress: "identity.test:8082", IdentitySessionServerName: "identity.test",
		IdentityClientCertFile: certFile + ".missing", IdentityClientKeyFile: keyFile, IdentityCAFile: caFile,
	}, zap.NewNop()); err == nil {
		t.Fatal("NewServer() accepted missing client certificate")
	}
	assertNoDatabaseConnections(t, container, "workspace-api-failed")

	address := freeAddress(t)
	core, logs := observer.New(zap.InfoLevel)
	logger := zap.New(core, zap.Fields(zap.String("service", "workspace-api"), zap.String("environment", "integration")))
	server, err := NewServer(ctx, Config{
		Environment:     "integration",
		HTTPAddress:     address,
		DatabaseURL:     sharedconfig.Secret(databaseURL),
		IdentityJWKSURL: jwksURL,
		IdentityIssuer:  integrationIssuer, IdentityAudience: "flowspace-api",
		IdentitySessionAddress: "identity.test:8082", IdentitySessionServerName: "identity.test",
		IdentityClientCertFile: certFile, IdentityClientKeyFile: keyFile, IdentityCAFile: caFile,
	}, logger)
	if err != nil {
		t.Fatal(err)
	}

	runContext, cancel := context.WithCancel(ctx)
	result := make(chan error, 1)
	go func() { result <- server.Run(runContext) }()
	waitForServer(t, result, "http://"+address+"/v1/workspaces/workspace-1")
	cancel()
	select {
	case err := <-result:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(serverTimeout):
		t.Fatal("server did not stop before the shutdown deadline")
	}

	entries := logs.FilterMessage("process_listening").AllUntimed()
	if len(entries) != 1 || entries[0].ContextMap()["service"] != "workspace-api" || entries[0].ContextMap()["environment"] != "integration" {
		t.Fatalf("process_listening logs = %v", entries)
	}
	assertNoDatabaseConnections(t, container, "workspace-api")
}

func testJWKSURL(t *testing.T) string {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		if err := json.NewEncoder(response).Encode(map[string]any{"keys": []any{}}); err != nil {
			t.Error(err)
		}
	}))
	t.Cleanup(server.Close)
	return server.URL
}

func testIdentityTLSFiles(t *testing.T) (string, string, string, tls.Certificate, *x509.CertPool) {
	t.Helper()
	_, caKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	caTemplate := &x509.Certificate{
		SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "test CA"},
		NotBefore: now.Add(-time.Hour), NotAfter: now.Add(time.Hour), IsCA: true,
		BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign,
	}
	caDER, err := x509.CreateCertificate(rand.Reader, caTemplate, caTemplate, caKey.Public(), caKey)
	if err != nil {
		t.Fatal(err)
	}
	ca, err := x509.ParseCertificate(caDER)
	if err != nil {
		t.Fatal(err)
	}
	issue := func(serial int64, usage x509.ExtKeyUsage) (tls.Certificate, []byte) {
		t.Helper()
		_, key, err := ed25519.GenerateKey(rand.Reader)
		if err != nil {
			t.Fatal(err)
		}
		template := &x509.Certificate{
			SerialNumber: big.NewInt(serial), DNSNames: []string{"identity.test"},
			NotBefore: now.Add(-time.Hour), NotAfter: now.Add(time.Hour),
			KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{usage},
		}
		der, err := x509.CreateCertificate(rand.Reader, template, ca, key.Public(), caKey)
		if err != nil {
			t.Fatal(err)
		}
		keyDER, err := x509.MarshalPKCS8PrivateKey(key)
		if err != nil {
			t.Fatal(err)
		}
		return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key}, keyDER
	}
	client, keyDER := issue(2, x509.ExtKeyUsageClientAuth)
	server, _ := issue(3, x509.ExtKeyUsageServerAuth)
	directory := t.TempDir()
	certFile, keyFile, caFile := filepath.Join(directory, "client.crt"), filepath.Join(directory, "client.key"), filepath.Join(directory, "ca.crt")
	for path, content := range map[string][]byte{
		certFile: pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: client.Certificate[0]}),
		keyFile:  pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER}),
		caFile:   pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: caDER}),
	} {
		if err := os.WriteFile(path, content, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	pool := x509.NewCertPool()
	pool.AddCert(ca)
	return certFile, keyFile, caFile, server, pool
}

func freeAddress(t *testing.T) string {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	address := listener.Addr().String()
	if err := listener.Close(); err != nil {
		t.Fatal(err)
	}
	return address
}

func waitForServer(t *testing.T, result <-chan error, url string) {
	t.Helper()
	client := &http.Client{Timeout: 100 * time.Millisecond}
	deadline := time.Now().Add(serverTimeout)
	for time.Now().Before(deadline) {
		select {
		case err := <-result:
			t.Fatalf("server stopped before it accepted requests: %v", err)
		default:
		}
		response, err := client.Get(url)
		if err == nil {
			_ = response.Body.Close()
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("server did not accept requests before the startup deadline")
}

func assertNoDatabaseConnections(t *testing.T, container *postgrescontainer.PostgresContainer, applicationName string) {
	t.Helper()
	ctx := t.Context()
	databaseURL, err := container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatal(err)
	}
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	var count int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM pg_stat_activity WHERE application_name = $1", applicationName).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("database connections for %q = %d, want 0", applicationName, count)
	}
}
