package main_test

/*
 * This file implements a certificate authority and certs for testing otel-cli's
 * TLS settings.
 *
 * Do NOT copy this code for production systems. It makes a few compromises to
 * optimize for testing and ephemeral certs that are totally inappropriate for
 * use in settings where security matters.
 */

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
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

	expire := time.Now().Add(time.Hour)

	// ------------- CA -------------

	ca := &x509.Certificate{
		SerialNumber:          big.NewInt(4317),
		NotBefore:             time.Now(),
		NotAfter:              expire,
		IsCA:                  true,
		BasicConstraintsValid: true,
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

	serverCert := &x509.Certificate{
		SerialNumber: big.NewInt(4318),
		IPAddresses:  []net.IP{net.IPv4(127, 0, 0, 1), net.IPv6loopback},
		NotBefore:    time.Now(),
		NotAfter:     expire,
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

	out.serverTLSConf = &tls.Config{
		ClientCAs:    out.certpool,
		Certificates: []tls.Certificate{serverCertPair},
	}

	// ------------- client -------------

	clientCert := &x509.Certificate{
		SerialNumber: big.NewInt(4319),
		NotBefore:    time.Now(),
		NotAfter:     expire,
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
