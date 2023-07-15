package otlpclient

import (
	"crypto/tls"
	"crypto/x509"
	"os"
)

// TlsConfig evaluates otel-cli configuration and returns a tls.Config
// that can be used by grpc or https.
func TlsConfig(config Config) *tls.Config {
	tlsConfig := &tls.Config{}

	if config.TlsNoVerify {
		Diag.InsecureSkipVerify = true
		tlsConfig.InsecureSkipVerify = true
	}

	// puts the provided CA certificate into the root pool
	// when not provided, Go TLS will automatically load the system CA pool
	if config.TlsCACert != "" {
		data, err := os.ReadFile(config.TlsCACert)
		if err != nil {
			config.SoftFail("failed to load CA certificate: %s", err)
		}

		certpool := x509.NewCertPool()
		certpool.AppendCertsFromPEM(data)
		tlsConfig.RootCAs = certpool
	}

	// client certificate authentication
	if config.TlsClientCert != "" && config.TlsClientKey != "" {
		clientPEM, err := os.ReadFile(config.TlsClientCert)
		if err != nil {
			config.SoftFail("failed to read client certificate file %s: %s", config.TlsClientCert, err)
		}
		clientKeyPEM, err := os.ReadFile(config.TlsClientKey)
		if err != nil {
			config.SoftFail("failed to read client key file %s: %s", config.TlsClientKey, err)
		}
		certPair, err := tls.X509KeyPair(clientPEM, clientKeyPEM)
		if err != nil {
			config.SoftFail("failed to parse client cert pair: %s", err)
		}
		tlsConfig.Certificates = []tls.Certificate{certPair}
	} else if config.TlsClientCert != "" {
		config.SoftFail("client cert and key must be specified together")
	} else if config.TlsClientKey != "" {
		config.SoftFail("client cert and key must be specified together")
	}

	return tlsConfig
}
