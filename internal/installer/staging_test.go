// internal/installer/staging_test.go

package installer

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"testing"
	"time"
)

// certParses guards the TLS-cert stager against copying a
// half-written file: only a complete, parseable certificate
// may reach the board.
func TestCertParses(t *testing.T) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "test"},
		NotBefore:    time.Now(),
		NotAfter:     time.Now().Add(time.Hour),
	}
	der, err := x509.CreateCertificate(
		rand.Reader, &tmpl, &tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	full := pem.EncodeToMemory(
		&pem.Block{Type: "CERTIFICATE", Bytes: der})

	if !certParses(full) {
		t.Error("valid certificate rejected")
	}
	if certParses(full[:len(full)/2]) {
		t.Error("truncated certificate accepted")
	}
	if certParses([]byte("not a pem")) {
		t.Error("junk accepted")
	}
	if certParses(nil) {
		t.Error("empty input accepted")
	}
}
