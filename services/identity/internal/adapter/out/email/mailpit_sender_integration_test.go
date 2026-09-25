//go:build integration

package email_test

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"

	identityemail "github.com/vasapolrittideah/flowspace-api/services/identity/internal/adapter/out/email"
)

func TestMailpitSender(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	mailpit, err := testcontainers.Run(ctx, "axllent/mailpit:v1.31.1",
		testcontainers.WithExposedPorts("1025/tcp", "8025/tcp"),
		testcontainers.WithWaitStrategy(wait.ForHTTP("/api/v1/info").WithPort("8025/tcp")))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = testcontainers.TerminateContainer(mailpit) })
	host, err := mailpit.Host(ctx)
	if err != nil {
		t.Fatal(err)
	}
	smtpPort, err := mailpit.MappedPort(ctx, "1025/tcp")
	if err != nil {
		t.Fatal(err)
	}
	httpPort, err := mailpit.MappedPort(ctx, "8025/tcp")
	if err != nil {
		t.Fatal(err)
	}
	smtpAddress := net.JoinHostPort(host, smtpPort.Port())
	httpAddress := net.JoinHostPort(host, httpPort.Port())
	sender, err := identityemail.NewMailpitSender(smtpAddress, "no-reply@flowspace.local")
	if err != nil {
		t.Fatal(err)
	}
	if err := sender.Send(ctx, "Recipient@example.com", "123456", "verify-email"); err != nil {
		t.Fatalf("Mailpit send failed: %v", err)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://"+httpAddress+"/api/v1/message/latest/raw", nil)
	if err != nil {
		t.Fatal(err)
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = response.Body.Close() }()
	raw, err := io.ReadAll(response.Body)
	if err != nil || response.StatusCode != http.StatusOK || !strings.Contains(string(raw), "Recipient@example.com") || !strings.Contains(string(raw), "123456") {
		t.Fatalf("Mailpit did not capture the expected message: status %d, error %v", response.StatusCode, err)
	}
	deadSender, err := identityemail.NewMailpitSender("127.0.0.1:1", "no-reply@flowspace.local")
	if err != nil {
		t.Fatal(err)
	}
	err = deadSender.Send(ctx, "Recipient@example.com", "123456", "verify-email")
	if !errors.Is(err, identityemail.ErrMailDelivery) || strings.Contains(err.Error(), "Recipient@example.com") || strings.Contains(err.Error(), "123456") {
		t.Fatalf("unsafe mail failure: %v", err)
	}
}
