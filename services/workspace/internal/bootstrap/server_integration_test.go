//go:build integration

package bootstrap

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/testcontainers/testcontainers-go"
	postgrescontainer "github.com/testcontainers/testcontainers-go/modules/postgres"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"

	sharedconfig "github.com/vasapolrittideah/flowspace-api/internal/config"
)

const integrationIssuer = "https://identity.test/realms/flowspace"

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
	discoveryURL := newOIDCDiscoveryServer(t)
	failedDatabaseURL, err := container.ConnectionString(ctx, "sslmode=disable", "application_name=workspace-api-failed")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := NewServer(ctx, Config{
		HTTPAddress:      "127.0.0.1:0",
		DatabaseURL:      sharedconfig.Secret(failedDatabaseURL),
		OIDCDiscoveryURL: discoveryURL + "/missing",
		OIDCIssuer:       integrationIssuer,
		OIDCAudience:     "workspace-api",
	}, zap.NewNop()); err == nil {
		t.Fatal("NewServer() accepted failed OIDC discovery")
	}
	assertNoDatabaseConnections(t, container, "workspace-api-failed")

	address := freeAddress(t)
	core, logs := observer.New(zap.InfoLevel)
	logger := zap.New(core, zap.Fields(zap.String("service", "workspace-api"), zap.String("environment", "integration")))
	server, err := NewServer(ctx, Config{
		Environment:      "integration",
		HTTPAddress:      address,
		DatabaseURL:      sharedconfig.Secret(databaseURL),
		OIDCDiscoveryURL: discoveryURL,
		OIDCIssuer:       integrationIssuer,
		OIDCAudience:     "workspace-api",
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

func newOIDCDiscoveryServer(t *testing.T) string {
	t.Helper()
	var serverURL string
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/.well-known/openid-configuration" {
			http.NotFound(response, request)
			return
		}
		response.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(response).Encode(map[string]any{
			"issuer":                                integrationIssuer,
			"jwks_uri":                              serverURL + "/keys",
			"id_token_signing_alg_values_supported": []string{"RS256"},
		}); err != nil {
			t.Error(err)
		}
	}))
	serverURL = server.URL
	t.Cleanup(server.Close)
	return serverURL
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
