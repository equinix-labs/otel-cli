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
	caFile            string
	caPrivKeyFile     string
	serverFile        string
	serverPrivKeyFile string
	clientFile        string
	clientPrivKeyFile string
	serverTLSConf     *tls.Config
	clientTLSConf     *tls.Config
	certpool          *x509.CertPool
}

func generateTLSData(t *testing.T) tlsHelpers {
	var err error
	var out tlsHelpers

	// this gets reused for each cert, with CommonName overwritten for each
	subject := pkix.Name{
		CommonName:    "otel-cli certificate authority",
		Organization:  []string{"otel-cli testing, inc"},
		Country:       []string{"Open Source"},
		Province:      []string{"Go"},
		Locality:      []string{"OpenTelemetry"},
		StreetAddress: []string{"github.com/equinix-labs/otel-cli"},
		PostalCode:    []string{"4317"},
	}

	expire := time.Now().Add(time.Hour * 1000)

	// ------------- CA -------------

	ca := &x509.Certificate{
		SerialNumber:          big.NewInt(4317),
		Subject:               subject,
		NotBefore:             time.Now(),
		NotAfter:              expire,
		IsCA:                  true,
		BasicConstraintsValid: true,
		ExtKeyUsage: []x509.ExtKeyUsage{
			x509.ExtKeyUsageClientAuth,
			x509.ExtKeyUsageServerAuth,
			x509.ExtKeyUsageOCSPSigning,
		},
		KeyUsage: x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
	}

	// create a private key
	caPrivKey, err := ecdsa.GenerateKey(elliptic.P521(), rand.Reader)
	if err != nil {
		t.Fatalf("error generating ca private key: %s", err)
	}

	// create a cert on the CA with the ^^ private key
	caBytes, err := x509.CreateCertificate(rand.Reader, ca, ca, &caPrivKey.PublicKey, caPrivKey)
	if err != nil {
		t.Fatalf("error generating ca cert: %s", err)
	}

	// get the PEM encoding that the tests will use
	caPEM := new(bytes.Buffer)
	pem.Encode(caPEM, &pem.Block{Type: "CERTIFICATE", Bytes: caBytes})
	out.caFile = pemToTempFile(t, "ca-cert", caPEM)

	caPrivKeyPEM := new(bytes.Buffer)
	caPrivKeyBytes, err := x509.MarshalECPrivateKey(caPrivKey)
	if err != nil {
		t.Fatalf("error marshaling server cert: %s", err)
	}
	pem.Encode(caPrivKeyPEM, &pem.Block{Type: "EC PRIVATE KEY", Bytes: caPrivKeyBytes})
	out.caPrivKeyFile = pemToTempFile(t, "ca-privkey", caPrivKeyPEM)

	out.certpool = x509.NewCertPool()
	out.certpool.AppendCertsFromPEM(caPEM.Bytes())

	data := new(bytes.Buffer)
	pem.Encode(data, &pem.Block{Type: "EC PRIVATE KEY", Bytes: caPrivKeyBytes})

	// ------------- server -------------

	subject.CommonName = "server"
	serverCert := &x509.Certificate{
		SerialNumber: big.NewInt(4318),
		Subject:      subject,
		IPAddresses:  []net.IP{net.IPv4(127, 0, 0, 1), net.IPv6loopback},
		DNSNames:     []string{"localhost"},
		NotBefore:    time.Now(),
		NotAfter:     expire,
		ExtKeyUsage: []x509.ExtKeyUsage{
			x509.ExtKeyUsageClientAuth,
			x509.ExtKeyUsageServerAuth,
		},
		KeyUsage: x509.KeyUsageKeyAgreement | x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
	}

	serverPrivKey, err := ecdsa.GenerateKey(elliptic.P521(), rand.Reader)
	if err != nil {
		t.Fatalf("error generating server private key: %s", err)
	}

	serverBytes, err := x509.CreateCertificate(rand.Reader, serverCert, ca, &serverPrivKey.PublicKey, caPrivKey)
	if err != nil {
		t.Fatalf("error generating server cert: %s", err)
	}

	serverPEM := new(bytes.Buffer)
	pem.Encode(serverPEM, &pem.Block{Type: "CERTIFICATE", Bytes: serverBytes})
	out.serverFile = pemToTempFile(t, "server-cert", serverPEM)

	serverPrivKeyPEM := new(bytes.Buffer)
	serverPrivKeyBytes, err := x509.MarshalECPrivateKey(serverPrivKey)
	if err != nil {
		t.Fatalf("error marshaling server cert: %s", err)
	}
	pem.Encode(serverPrivKeyPEM, &pem.Block{Type: "EC PRIVATE KEY", Bytes: serverPrivKeyBytes})
	out.serverPrivKeyFile = pemToTempFile(t, "server-privkey", serverPrivKeyPEM)

	serverCertPair, err := tls.X509KeyPair(serverPEM.Bytes(), serverPrivKeyPEM.Bytes())
	if err != nil {
		t.Fatalf("error generating server cert pair: %s", err)
	}

	// In theory, the server shouldn't need this CA in the RootCAs pool to accept client
	// connections. Without it, grpc refuses the client connection with invalid CA.
	// No amount of client config changes would work. The opentelemetry collector also sets
	// RootCAs by default so it seems safe to copy that behavior here.
	out.serverTLSConf = &tls.Config{
		RootCAs:      out.certpool,
		ServerName:   "localhost",
		ClientCAs:    out.certpool,
		Certificates: []tls.Certificate{serverCertPair},
	}

	// ------------- client -------------

	subject.CommonName = "client"
	clientCert := &x509.Certificate{
		SerialNumber: big.NewInt(4319),
		Subject:      subject,
		DNSNames:     []string{"localhost"},
		NotBefore:    time.Now(),
		NotAfter:     expire,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageAny},
		KeyUsage:     x509.KeyUsageDigitalSignature,
	}

	clientPrivKey, err := ecdsa.GenerateKey(elliptic.P521(), rand.Reader)
	if err != nil {
		t.Fatalf("error generating client private key: %s", err)
	}

	clientBytes, err := x509.CreateCertificate(rand.Reader, clientCert, ca, &clientPrivKey.PublicKey, caPrivKey)
	if err != nil {
		t.Fatalf("error generating client cert: %s", err)
	}

	clientPEM := new(bytes.Buffer)
	pem.Encode(clientPEM, &pem.Block{Type: "CERTIFICATE", Bytes: clientBytes})
	out.clientFile = pemToTempFile(t, "client-cert", clientPEM)

	clientPrivKeyPEM := new(bytes.Buffer)
	clientPrivKeyBytes, err := x509.MarshalECPrivateKey(clientPrivKey)
	if err != nil {
		t.Fatalf("error marshaling client cert: %s", err)
	}
	pem.Encode(clientPrivKeyPEM, &pem.Block{Type: "EC PRIVATE KEY", Bytes: clientPrivKeyBytes})
	out.clientPrivKeyFile = pemToTempFile(t, "client-privkey", clientPrivKeyPEM)

	out.clientTLSConf = &tls.Config{
		ServerName: "localhost",
	}

	return out
}

func (t tlsHelpers) cleanup() {
	os.Remove(t.caFile)
	os.Remove(t.caPrivKeyFile)
	os.Remove(t.clientFile)
	os.Remove(t.clientPrivKeyFile)
	os.Remove(t.serverFile)
	os.Remove(t.serverPrivKeyFile)
}

func pemToTempFile(t *testing.T, tmpl string, buf *bytes.Buffer) string {
	tmp, err := os.CreateTemp(os.TempDir(), "otel-cli-test-"+tmpl+"-pem")
	if err != nil {
		t.Fatalf("error creating temp file: %s", err)
	}
	tmp.Write(buf.Bytes())
	tmp.Close()
	return tmp.Name()
}
