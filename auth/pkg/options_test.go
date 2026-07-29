package auth

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"strings"
	"testing"
)

func testKeyPEM(t *testing.T) string {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("GenerateKey returned error: %v", err)
	}
	block := &pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)}
	return string(pem.EncodeToMemory(block))
}

func TestNormalizePEMAcceptsRealNewlines(t *testing.T) {
	want := testKeyPEM(t)

	got := NormalizePEM(want)

	if block, _ := pem.Decode(got); block == nil {
		t.Fatalf("NormalizePEM output does not decode as PEM: %q", got)
	}
}

func TestNormalizePEMAcceptsEscapedNewlines(t *testing.T) {
	escaped := strings.ReplaceAll(strings.TrimSpace(testKeyPEM(t)), "\n", `\n`)

	got := NormalizePEM(escaped)

	block, _ := pem.Decode(got)
	if block == nil {
		t.Fatalf("NormalizePEM did not restore newlines: %q", got)
	}
	if _, err := x509.ParsePKCS1PrivateKey(block.Bytes); err != nil {
		t.Fatalf("ParsePKCS1PrivateKey returned error: %v", err)
	}
}

func TestNormalizePEMStripsCarriageReturns(t *testing.T) {
	windows := strings.ReplaceAll(testKeyPEM(t), "\n", "\r\n")

	got := NormalizePEM(windows)

	if strings.Contains(string(got), "\r") {
		t.Fatal("NormalizePEM kept carriage returns")
	}
	if block, _ := pem.Decode(got); block == nil {
		t.Fatalf("NormalizePEM output does not decode as PEM: %q", got)
	}
}
