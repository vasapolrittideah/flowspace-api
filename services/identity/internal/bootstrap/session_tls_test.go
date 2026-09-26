package bootstrap

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/pem"
	"math/big"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const sessionTestServerName = "identity.test"

type sessionTLSFixture struct {
	config  APIConfig
	roots   *x509.CertPool
	clients map[string]tls.Certificate
}

func testSessionCertificate(t *testing.T, template, parent *x509.Certificate, parentKey ed25519.PrivateKey) tls.Certificate {
	t.Helper()
	_, key, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	if parent == nil {
		parent, parentKey = template, key
	}
	der, err := x509.CreateCertificate(rand.Reader, template, parent, key.Public(), parentKey)
	if err != nil {
		t.Fatal(err)
	}
	leaf, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key, Leaf: leaf}
}

func testSessionPrivateKey(t *testing.T, certificate tls.Certificate) ed25519.PrivateKey {
	t.Helper()
	key, ok := certificate.PrivateKey.(ed25519.PrivateKey)
	if !ok {
		t.Fatal("test certificate has a different key type")
	}
	return key
}

func testSessionTLSFixture(t *testing.T) sessionTLSFixture {
	t.Helper()
	now := time.Now()
	caTemplate := &x509.Certificate{
		SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "test identity CA"},
		NotBefore: now.Add(-24 * time.Hour), NotAfter: now.Add(24 * time.Hour),
		IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign,
	}
	ca := testSessionCertificate(t, caTemplate, nil, nil)
	otherCA := testSessionCertificate(t, caTemplate, nil, nil)
	workspaceURI, err := url.Parse("urn:flowspace:service:workspace")
	if err != nil {
		t.Fatal(err)
	}
	wrongURI, err := url.Parse("urn:flowspace:service:other")
	if err != nil {
		t.Fatal(err)
	}
	server := testSessionCertificate(t, &x509.Certificate{
		SerialNumber: big.NewInt(2), DNSNames: []string{sessionTestServerName},
		NotBefore: now.Add(-time.Hour), NotAfter: now.Add(time.Hour),
		KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}, ca.Leaf, testSessionPrivateKey(t, ca))
	clients := map[string]tls.Certificate{}
	for index, name := range []string{"approved", "rotated", "wrong-service", "wrong-key", "expired", "untrusted", "wrong-use", "extra-uri"} {
		template := &x509.Certificate{
			SerialNumber: big.NewInt(int64(index + 3)), URIs: []*url.URL{workspaceURI},
			NotBefore: now.Add(-time.Hour), NotAfter: now.Add(time.Hour),
			KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
		}
		issuer := ca
		switch name {
		case "wrong-service":
			template.URIs = []*url.URL{wrongURI}
		case "expired":
			template.NotBefore, template.NotAfter = now.Add(-2*time.Hour), now.Add(-time.Hour)
		case "untrusted":
			issuer = otherCA
		case "wrong-use":
			template.ExtKeyUsage = []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}
		case "extra-uri":
			template.URIs = []*url.URL{workspaceURI, wrongURI}
		}
		clients[name] = testSessionCertificate(t, template, issuer.Leaf, testSessionPrivateKey(t, issuer))
	}
	directory := t.TempDir()
	certFile := filepath.Join(directory, "server.pem")
	keyFile := filepath.Join(directory, "server-key.pem")
	caFile := filepath.Join(directory, "ca.pem")
	keyDER, err := x509.MarshalPKCS8PrivateKey(server.PrivateKey)
	if err != nil {
		t.Fatal(err)
	}
	for path, data := range map[string][]byte{
		certFile: pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: server.Certificate[0]}),
		keyFile:  pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER}),
		caFile:   pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: ca.Certificate[0]}),
	} {
		if err := os.WriteFile(path, data, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	roots := x509.NewCertPool()
	roots.AddCert(ca.Leaf)
	fingerprint := func(name string) string {
		value := sha256.Sum256(clients[name].Leaf.RawSubjectPublicKeyInfo)
		return hex.EncodeToString(value[:])
	}
	return sessionTLSFixture{
		config: APIConfig{
			HTTPAddress: ":8080", InternalHTTPAddress: ":8081", SessionGRPCAddress: ":8082",
			SessionTLSCertFile: certFile, SessionTLSKeyFile: keyFile, SessionClientCAFile: caFile,
			SessionCallerAllowlist: "urn:flowspace:service:workspace=" + fingerprint("approved") +
				",urn:flowspace:service:workspace=" + fingerprint("rotated"),
		},
		roots: roots, clients: clients,
	}
}

func testSessionHandshake(ctx context.Context, serverConfig, clientConfig *tls.Config) (error, error) {
	serverConn, clientConn := net.Pipe()
	defer func() { _ = serverConn.Close() }()
	defer func() { _ = clientConn.Close() }()
	_ = serverConn.SetDeadline(time.Now().Add(5 * time.Second))
	_ = clientConn.SetDeadline(time.Now().Add(5 * time.Second))
	serverResult := make(chan error, 1)
	go func() {
		serverResult <- tls.Server(serverConn, serverConfig).HandshakeContext(ctx)
		_ = serverConn.Close()
	}()
	clientErr := tls.Client(clientConn, clientConfig).HandshakeContext(ctx)
	_ = clientConn.Close()
	return <-serverResult, clientErr
}

func TestSessionRPCConfiguration(t *testing.T) {
	if enabled, err := sessionRPCEnabled(APIConfig{}); err != nil || enabled {
		t.Fatalf("unconfigured listener = %t, %v", enabled, err)
	}
	config := APIConfig{
		HTTPAddress: ":8080", InternalHTTPAddress: ":8081", SessionGRPCAddress: ":8082",
		SessionTLSCertFile: "server.pem", SessionTLSKeyFile: "server-key.pem", SessionClientCAFile: "ca.pem",
		SessionCallerAllowlist: "urn:flowspace:service:workspace=aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
	}
	if enabled, err := sessionRPCEnabled(config); err != nil || !enabled {
		t.Fatalf("complete listener = %t, %v", enabled, err)
	}
	for _, field := range []string{"address", "cert", "key", "ca", "allowlist", "public collision", "health collision"} {
		candidate := config
		switch field {
		case "address":
			candidate.SessionGRPCAddress = ""
		case "cert":
			candidate.SessionTLSCertFile = ""
		case "key":
			candidate.SessionTLSKeyFile = ""
		case "ca":
			candidate.SessionClientCAFile = ""
		case "allowlist":
			candidate.SessionCallerAllowlist = ""
		case "public collision":
			candidate.SessionGRPCAddress = config.HTTPAddress
		case "health collision":
			candidate.SessionGRPCAddress = config.InternalHTTPAddress
		}
		if _, err := sessionRPCEnabled(candidate); err == nil {
			t.Fatalf("accepted %s", field)
		}
	}
}

func TestSessionTLSClientAuthentication(t *testing.T) {
	fixture := testSessionTLSFixture(t)
	serverConfig, err := newSessionTLSConfig(fixture.config)
	if err != nil || serverConfig.MinVersion != tls.VersionTLS13 || serverConfig.ClientAuth != tls.RequireAndVerifyClientCert {
		t.Fatalf("private TLS configuration = %+v, %v", serverConfig, err)
	}
	for _, name := range []string{"approved", "rotated", "missing", "expired", "untrusted", "wrong-service", "wrong-key", "wrong-use", "extra-uri", "tls-1.2"} {
		t.Run(name, func(t *testing.T) {
			clientConfig := &tls.Config{RootCAs: fixture.roots, ServerName: sessionTestServerName, MinVersion: tls.VersionTLS13}
			if client, ok := fixture.clients[name]; ok {
				clientConfig.Certificates = []tls.Certificate{client}
			}
			if name == "tls-1.2" {
				clientConfig.MinVersion, clientConfig.MaxVersion = tls.VersionTLS12, tls.VersionTLS12
			}
			serverErr, clientErr := testSessionHandshake(t.Context(), serverConfig, clientConfig)
			allowed := name == "approved" || name == "rotated"
			if allowed && (serverErr != nil || clientErr != nil) || !allowed && serverErr == nil {
				t.Fatalf("handshake server = %v, client = %v", serverErr, clientErr)
			}
		})
	}
}

func TestSessionCallerAllowlistRejectsInvalidEntries(t *testing.T) {
	fixture := testSessionTLSFixture(t)
	for _, allowlist := range []string{"", "wrong", "urn:flowspace:service:workspace=bad", "urn:other=aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"} {
		config := fixture.config
		config.SessionCallerAllowlist = allowlist
		if _, err := newSessionTLSConfig(config); err == nil || strings.Contains(err.Error(), fixture.config.SessionTLSKeyFile) {
			t.Fatalf("accepted or disclosed invalid allowlist %q: %v", allowlist, err)
		}
	}
}

func TestSessionTLSRejectsInvalidFiles(t *testing.T) {
	fixture := testSessionTLSFixture(t)
	invalidFile := filepath.Join(t.TempDir(), "invalid.pem")
	if err := os.WriteFile(invalidFile, []byte("sensitive-invalid-pem"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{"certificate", "key", "CA"} {
		config := fixture.config
		switch field {
		case "certificate":
			config.SessionTLSCertFile = invalidFile
		case "key":
			config.SessionTLSKeyFile = invalidFile
		case "CA":
			config.SessionClientCAFile = invalidFile
		}
		if _, err := newSessionTLSConfig(config); err == nil || strings.Contains(err.Error(), "sensitive-invalid-pem") ||
			strings.Contains(err.Error(), invalidFile) {
			t.Fatalf("accepted or disclosed invalid %s: %v", field, err)
		}
	}
}
