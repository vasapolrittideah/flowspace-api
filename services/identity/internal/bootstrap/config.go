package bootstrap

import (
	"crypto/ed25519"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"os"
	"path/filepath"

	sharedconfig "github.com/vasapolrittideah/flowspace-api/internal/config"
)

type APIConfig struct {
	Environment            string              `env:"ENVIRONMENT,required,notEmpty"`
	HTTPAddress            string              `env:"HTTP_ADDR"                                envDefault:":8080"`
	InternalHTTPAddress    string              `env:"INTERNAL_HTTP_ADDR"                       envDefault:":8081"`
	SessionGRPCAddress     string              `env:"SESSION_GRPC_ADDR"`
	SessionTLSCertFile     string              `env:"SESSION_TLS_CERT_FILE"`
	SessionTLSKeyFile      string              `env:"SESSION_TLS_KEY_FILE"`
	SessionClientCAFile    string              `env:"SESSION_CLIENT_CA_FILE"`
	SessionCallerAllowlist string              `env:"SESSION_CALLER_ALLOWLIST"`
	DatabaseURL            sharedconfig.Secret `env:"DATABASE_URL,required,notEmpty"`
	SigningKeyFile         string              `env:"SIGNING_KEY_FILE,required,notEmpty"`
	SigningKeyID           string              `env:"SIGNING_KEY_ID,required,notEmpty"`
	TokenIssuer            string              `env:"TOKEN_ISSUER,required,notEmpty"`
	TokenAudience          string              `env:"TOKEN_AUDIENCE,required,notEmpty"`
	CodeVerifierKeyFile    string              `env:"CODE_VERIFIER_KEY_FILE,required,notEmpty"`
	DeliveryKeyFile        string              `env:"DELIVERY_KEY_FILE,required,notEmpty"`
	TrustedProxyCIDRs      string              `env:"TRUSTED_PROXY_CIDRS"`
	OutboxReadyMaxPending  int                 `env:"OUTBOX_READY_MAX_PENDING"                 envDefault:"10000"`
}

type WorkerConfig struct {
	Environment       string              `env:"ENVIRONMENT,required,notEmpty"`
	HealthAddress     string              `env:"HEALTH_ADDR"                           envDefault:":8081"`
	DatabaseURL       sharedconfig.Secret `env:"DATABASE_URL,required,notEmpty"`
	DeliveryKeyFile   string              `env:"DELIVERY_KEY_FILE,required,notEmpty"`
	BrokerAddress     string              `env:"BROKER_ADDR,required,notEmpty"`
	BrokerUsername    string              `env:"BROKER_USERNAME"`
	BrokerPassword    sharedconfig.Secret `env:"BROKER_PASSWORD"`
	SchemaRegistryURL string              `env:"SCHEMA_REGISTRY_URL,required,notEmpty"`
	DeliveryTopic     string              `env:"DELIVERY_TOPIC,required,notEmpty"`
	DeliveryGroup     string              `env:"DELIVERY_GROUP,required,notEmpty"`
	MailAddress       string              `env:"MAIL_ADDR,required,notEmpty"`
	MailFrom          string              `env:"MAIL_FROM,required,notEmpty"`
}

func LoadAPIConfig() (APIConfig, error) {
	config, err := sharedconfig.Load[APIConfig]()
	if err != nil {
		return APIConfig{}, errors.New("invalid API environment configuration")
	}
	if config.OutboxReadyMaxPending < 1 || config.HTTPAddress == config.InternalHTTPAddress {
		return APIConfig{}, errors.New("invalid API configuration")
	}
	if _, err := sessionRPCEnabled(config); err != nil {
		return APIConfig{}, err
	}
	if config.Environment == "local" && (config.TokenIssuer != "urn:flowspace:identity:local" || config.TokenAudience != "flowspace-api") {
		return APIConfig{}, errors.New("invalid local token configuration")
	}
	return config, nil
}

func sessionRPCEnabled(config APIConfig) (bool, error) {
	configured := 0
	for _, value := range []string{
		config.SessionGRPCAddress, config.SessionTLSCertFile, config.SessionTLSKeyFile,
		config.SessionClientCAFile, config.SessionCallerAllowlist,
	} {
		if value != "" {
			configured++
		}
	}
	if configured == 0 {
		return false, nil
	}
	if configured != 5 || config.SessionGRPCAddress == config.HTTPAddress || config.SessionGRPCAddress == config.InternalHTTPAddress {
		return false, errors.New("invalid session listener configuration")
	}
	return true, nil
}

func LoadWorkerConfig() (WorkerConfig, error) {
	config, err := sharedconfig.Load[WorkerConfig]()
	if err != nil {
		return WorkerConfig{}, errors.New("invalid worker environment configuration")
	}
	if (config.BrokerUsername == "") != (config.BrokerPassword == "") ||
		(config.Environment == "local" && config.BrokerUsername == "") {
		return WorkerConfig{}, errors.New("invalid broker credentials")
	}
	return config, nil
}

func readKey(path string) ([]byte, error) {
	key, err := readMountedFile(path)
	if err != nil || len(key) != 32 {
		return nil, errors.New("invalid key file")
	}
	return key, nil
}

func readSigningKey(path string) (ed25519.PrivateKey, error) {
	data, err := readMountedFile(path)
	if err != nil {
		return nil, errors.New("invalid signing key file")
	}
	block, rest := pem.Decode(data)
	if block == nil || block.Type != "PRIVATE KEY" || len(rest) != 0 {
		return nil, errors.New("invalid signing key file")
	}
	parsed, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	key, ok := parsed.(ed25519.PrivateKey)
	if err != nil || !ok || len(key) != ed25519.PrivateKeySize {
		return nil, errors.New("invalid signing key file")
	}
	return key, nil
}

func readMountedFile(path string) ([]byte, error) {
	root, err := os.OpenRoot(filepath.Dir(path))
	if err != nil {
		return nil, errors.New("invalid key file")
	}
	defer func() { _ = root.Close() }()
	content, err := root.ReadFile(filepath.Base(path))
	if err != nil {
		return nil, errors.New("invalid key file")
	}
	return content, nil
}
