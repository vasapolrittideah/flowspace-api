package bootstrap

import (
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"errors"
	"slices"
	"strings"
)

var errInvalidSessionTLS = errors.New("invalid session TLS configuration")

func newSessionTLSConfig(config APIConfig) (*tls.Config, error) {
	enabled, err := sessionRPCEnabled(config)
	if err != nil || !enabled {
		return nil, err
	}
	allowlist, err := parseSessionCallerAllowlist(config.SessionCallerAllowlist)
	if err != nil {
		return nil, errInvalidSessionTLS
	}
	certificate, err := readSessionCertificate(config.SessionTLSCertFile, config.SessionTLSKeyFile)
	if err != nil {
		return nil, errInvalidSessionTLS
	}
	caPEM, err := readMountedFile(config.SessionClientCAFile)
	if err != nil {
		return nil, errInvalidSessionTLS
	}
	clientCAs := x509.NewCertPool()
	if !clientCAs.AppendCertsFromPEM(caPEM) {
		return nil, errInvalidSessionTLS
	}
	return &tls.Config{
		MinVersion:   tls.VersionTLS13,
		Certificates: []tls.Certificate{certificate},
		ClientAuth:   tls.RequireAndVerifyClientCert,
		ClientCAs:    clientCAs,
		VerifyConnection: func(state tls.ConnectionState) error {
			if len(state.PeerCertificates) == 0 {
				return errInvalidSessionTLS
			}
			caller := state.PeerCertificates[0]
			if len(caller.URIs) != 1 || len(caller.DNSNames) != 0 || len(caller.EmailAddresses) != 0 || len(caller.IPAddresses) != 0 ||
				!slices.Contains(caller.ExtKeyUsage, x509.ExtKeyUsageClientAuth) {
				return errInvalidSessionTLS
			}
			fingerprint := sha256.Sum256(caller.RawSubjectPublicKeyInfo)
			if _, ok := allowlist[caller.URIs[0].String()][fingerprint]; !ok {
				return errInvalidSessionTLS
			}
			return nil
		},
	}, nil
}

func readSessionCertificate(certFile, keyFile string) (tls.Certificate, error) {
	certificatePEM, err := readMountedFile(certFile)
	if err != nil {
		return tls.Certificate{}, err
	}
	keyPEM, err := readMountedFile(keyFile)
	if err != nil {
		return tls.Certificate{}, err
	}
	return tls.X509KeyPair(certificatePEM, keyPEM)
}

func parseSessionCallerAllowlist(value string) (map[string]map[[32]byte]struct{}, error) {
	allowed := make(map[string]map[[32]byte]struct{})
	for entry := range strings.SplitSeq(value, ",") {
		identity, encoded, ok := strings.Cut(entry, "=")
		if !ok || !strings.HasPrefix(identity, "urn:flowspace:service:") || len(identity) == len("urn:flowspace:service:") || len(encoded) != 64 {
			return nil, errInvalidSessionTLS
		}
		decoded, err := hex.DecodeString(encoded)
		if err != nil {
			return nil, errInvalidSessionTLS
		}
		var fingerprint [32]byte
		copy(fingerprint[:], decoded)
		if allowed[identity] == nil {
			allowed[identity] = make(map[[32]byte]struct{})
		}
		allowed[identity][fingerprint] = struct{}{}
	}
	return allowed, nil
}
