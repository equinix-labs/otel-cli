package otelcli

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net"
	"net/url"
	"os"
)

// TlsConfig evaluates otel-cli configuration and returns a tls.Config
// that can be used by grpc or https.
func (config Config) GetTlsConfig() *tls.Config {
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

// GetInsecure returns true if the configuration expects a non-TLS connection.
func (c Config) GetInsecure() bool {
	endpointURL := c.GetEndpoint()

	isLoopback, err := isLoopbackAddr(endpointURL)
	c.SoftFailIfErr(err)

	// Go's TLS does the right thing and forces us to say we want to disable encryption,
	// but I expect most users of this program to point at a localhost endpoint that might not
	// have any encryption available, or setting it up raises the bar of entry too high.
	// The compromise is to automatically flip this flag to true when endpoint contains an
	// an obvious "localhost", "127.0.0.x", or "::1" address.
	if c.Insecure || (isLoopback && endpointURL.Scheme != "https") {
		return true
	} else if endpointURL.Scheme == "http" || endpointURL.Scheme == "unix" {
		return true
	}

	return false
}

// isLoopbackAddr takes a url.URL, looks up the address, then returns true
// if it points at either a v4 or v6 loopback address.
// As I understood the OTLP spec, only host:port or an HTTP URL are acceptable.
// This function is _not_ meant to validate the endpoint, that will happen when
// otel-go attempts to connect to the endpoint.
func isLoopbackAddr(u *url.URL) (bool, error) {
	hostname := u.Hostname()

	if hostname == "localhost" || hostname == "127.0.0.1" || hostname == "::1" {
		Diag.DetectedLocalhost = true
		return true, nil
	}

	ips, err := net.LookupIP(hostname)
	if err != nil {
		return false, fmt.Errorf("unable to look up hostname '%s': %s", hostname, err)
	}

	// all ips returned must be loopback to return true
	// cases where that isn't true should be super rare, and probably all shenanigans
	allAreLoopback := true
	for _, ip := range ips {
		if !ip.IsLoopback() {
			allAreLoopback = false
		}
	}

	Diag.DetectedLocalhost = allAreLoopback
	return allAreLoopback, nil
}
