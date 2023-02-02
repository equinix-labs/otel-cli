package main_test

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net"
	"os"
	"testing"
	"time"
)

type tlsHelpers struct {
	pool              string
	ca                *x509.Certificate
	caPrivKey         *ecdsa.PrivateKey
	caPEM             *bytes.Buffer
	caPrivKeyPEM      *bytes.Buffer
	caFile            string
	caPrivKeyFile     string
	serverCert        *x509.Certificate
	serverPrivKey     *ecdsa.PrivateKey
	serverPEM         *bytes.Buffer
	serverFile        string
	serverPrivKeyPEM  *bytes.Buffer
	serverPrivKeyFile string
	serverCertPair    tls.Certificate
	certpool          *x509.CertPool
	serverTLSConf     *tls.Config
	clientCert        *x509.Certificate
	clientPrivKey     *ecdsa.PrivateKey
	clientPEM         *bytes.Buffer
	clientFile        string
	clientPrivKeyPEM  *bytes.Buffer
	clientPrivKeyFile string
	clientTLSConf     *tls.Config
}

func generateTLSData(t *testing.T) tlsHelpers {
	var err error
	var out tlsHelpers

	// ------------- CA -------------

	out.ca = &x509.Certificate{
		SerialNumber: big.NewInt(4317),
		Subject: pkix.Name{
			Organization:  []string{"otel-cli testing, inc"},
			Country:       []string{"Open Source"},
			Province:      []string{"Go"},
			Locality:      []string{"OpenTelemetry"},
			StreetAddress: []string{"github.com/equinix-labs/otel-cli"},
			PostalCode:    []string{"4317"},
		},
		NotBefore:   time.Now(),
		NotAfter:    time.Now().Add(time.Hour),
		IsCA:        true,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageServerAuth},
		KeyUsage:    x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
	}

	// create a private key
	out.caPrivKey, err = ecdsa.GenerateKey(elliptic.P521(), rand.Reader)
	if err != nil {
		t.Fatalf("error generating ca private key: %s", err)
	}

	// create a cert on the CA with the ^^ private key
	caBytes, err := x509.CreateCertificate(rand.Reader, out.ca, out.ca, &out.caPrivKey.PublicKey, out.caPrivKey)
	if err != nil {
		t.Fatalf("error generating ca cert: %s", err)
	}

	// get the PEM encoding that the tests will use
	out.caPEM = new(bytes.Buffer)
	pem.Encode(out.caPEM, &pem.Block{Type: "CERTIFICATE", Bytes: caBytes})
	out.caFile = pemToTempFile(t, out.caPEM)

	out.caPrivKeyPEM = new(bytes.Buffer)
	caPrivKeyBytes, err := x509.MarshalECPrivateKey(out.caPrivKey)
	if err != nil {
		t.Fatalf("error marshaling server cert: %s", err)
	}
	pem.Encode(out.caPrivKeyPEM, &pem.Block{Type: "EC PRIVATE KEY", Bytes: caPrivKeyBytes})
	out.caPrivKeyFile = pemToTempFile(t, out.caPrivKeyPEM)

	out.certpool = x509.NewCertPool()
	out.certpool.AppendCertsFromPEM(out.caPEM.Bytes())

	data := new(bytes.Buffer)
	pem.Encode(data, &pem.Block{Type: "EC PRIVATE KEY", Bytes: caPrivKeyBytes})

	// ------------- server -------------

	out.serverCert = &x509.Certificate{
		SerialNumber: big.NewInt(4318),
		Subject:      out.ca.Subject,
		SubjectKeyId: []byte{1, 2, 3, 4, 6},
		IPAddresses:  []net.IP{net.IPv4(127, 0, 0, 1), net.IPv6loopback},
		DNSNames:     []string{"localhost"},
		NotBefore:    time.Now(),
		NotAfter:     time.Now().Add(time.Hour),
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageServerAuth},
		KeyUsage:     x509.KeyUsageDigitalSignature,
	}

	out.serverPrivKey, err = ecdsa.GenerateKey(elliptic.P521(), rand.Reader)
	if err != nil {
		t.Fatalf("error generating server private key: %s", err)
	}

	serverBytes, err := x509.CreateCertificate(rand.Reader, out.serverCert, out.ca, &out.serverPrivKey.PublicKey, out.caPrivKey)
	if err != nil {
		t.Fatalf("error generating server cert: %s", err)
	}

	out.serverPEM = new(bytes.Buffer)
	pem.Encode(out.serverPEM, &pem.Block{Type: "CERTIFICATE", Bytes: serverBytes})
	out.serverFile = pemToTempFile(t, out.serverPEM)

	out.serverPrivKeyPEM = new(bytes.Buffer)
	serverPrivKeyBytes, err := x509.MarshalECPrivateKey(out.serverPrivKey)
	if err != nil {
		t.Fatalf("error marshaling server cert: %s", err)
	}
	pem.Encode(out.serverPrivKeyPEM, &pem.Block{Type: "EC PRIVATE KEY", Bytes: serverPrivKeyBytes})
	out.serverPrivKeyFile = pemToTempFile(t, out.serverPrivKeyPEM)

	out.serverCertPair, err = tls.X509KeyPair(out.serverPEM.Bytes(), out.serverPrivKeyPEM.Bytes())
	if err != nil {
		t.Fatalf("error generating server cert pair: %s", err)
	}

	out.serverTLSConf = &tls.Config{
		RootCAs:      out.certpool,
		Certificates: []tls.Certificate{out.serverCertPair},
	}

	// ------------- client -------------

	out.clientCert = &x509.Certificate{
		SerialNumber: big.NewInt(4319),
		Subject:      out.ca.Subject,
		SubjectKeyId: []byte{1, 2, 3, 4, 6},
		IPAddresses:  []net.IP{net.IPv4(127, 0, 0, 1), net.IPv6loopback},
		DNSNames:     []string{"localhost"},
		NotBefore:    time.Now(),
		NotAfter:     time.Now().Add(time.Hour),
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageServerAuth},
		KeyUsage:     x509.KeyUsageDigitalSignature,
	}

	out.clientPrivKey, err = ecdsa.GenerateKey(elliptic.P521(), rand.Reader)
	if err != nil {
		t.Fatalf("error generating client private key: %s", err)
	}

	clientBytes, err := x509.CreateCertificate(rand.Reader, out.clientCert, out.ca, &out.clientPrivKey.PublicKey, out.caPrivKey)
	if err != nil {
		t.Fatalf("error generating client cert: %s", err)
	}

	out.clientPEM = new(bytes.Buffer)
	pem.Encode(out.clientPEM, &pem.Block{Type: "CERTIFICATE", Bytes: clientBytes})
	out.clientFile = pemToTempFile(t, out.clientPEM)

	out.clientPrivKeyPEM = new(bytes.Buffer)
	clientPrivKeyBytes, err := x509.MarshalECPrivateKey(out.clientPrivKey)
	if err != nil {
		t.Fatalf("error marshaling client cert: %s", err)
	}
	pem.Encode(out.clientPrivKeyPEM, &pem.Block{Type: "EC PRIVATE KEY", Bytes: clientPrivKeyBytes})
	out.clientPrivKeyFile = pemToTempFile(t, out.clientPrivKeyPEM)

	out.clientTLSConf = &tls.Config{
		RootCAs: out.certpool,
	}

	return out
}

func pemToTempFile(t *testing.T, buf *bytes.Buffer) string {
	tmp, err := os.CreateTemp(os.TempDir(), "otel-cli-test-pem")
	if err != nil {
		t.Fatalf("error creating temp file: %s", err)
	}
	tmp.Write(buf.Bytes())
	tmp.Close()
	return tmp.Name()
}
