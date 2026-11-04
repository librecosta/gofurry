package weather

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/json/v2"
	"encoding/pem"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/librecosta/gofurry/services/api/internal/platform"
)

func testConfig(t *testing.T) platform.QWeatherConfig {
	t.Helper()
	_, key, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal("generate test key")
	}
	der, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatal("encode test key")
	}
	path := filepath.Join(t.TempDir(), "test.pem")
	if err := os.WriteFile(path, pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der}), 0600); err != nil {
		t.Fatal("write test key")
	}
	return platform.QWeatherConfig{Enabled: true, APIHost: "test.qweatherapi.com", DeveloperID: "testDeveloper", ProjectID: "testProject", CredentialID: "testCredential", PrivateKeyPath: path}
}

func TestJWTSignatureClaimsRefresh(t *testing.T) {
	cfg := testConfig(t)
	source, err := newTokenSource(cfg)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Unix(1800000000, 0)
	source.now = func() time.Time { return now }
	first, err := source.bearer()
	if err != nil {
		t.Fatal(err)
	}
	parts := strings.Split(first, ".")
	if len(parts) != 3 || strings.Contains(first, "=") {
		t.Fatal("invalid compact Base64URL encoding")
	}
	header, _ := base64.RawURLEncoding.DecodeString(parts[0])
	payload, _ := base64.RawURLEncoding.DecodeString(parts[1])
	sig, _ := base64.RawURLEncoding.DecodeString(parts[2])
	var h struct {
		Alg string `json:"alg"`
		KID string `json:"kid"`
	}
	var claims struct {
		Iss string `json:"iss"`
		Sub string `json:"sub"`
		Iat int64  `json:"iat"`
		Exp int64  `json:"exp"`
	}
	if json.Unmarshal(header, &h) != nil || json.Unmarshal(payload, &claims) != nil {
		t.Fatal("invalid JWT JSON")
	}
	if h.Alg != "EdDSA" || h.KID != cfg.CredentialID || claims.Iss != cfg.DeveloperID || claims.Sub != cfg.ProjectID || claims.Iat != now.Unix()-30 || claims.Exp != claims.Iat+900 {
		t.Fatal("JWT claims mismatch")
	}
	if !ed25519.Verify(source.key.Public().(ed25519.PublicKey), []byte(parts[0]+"."+parts[1]), sig) {
		t.Fatal("signature did not verify")
	}
	if ed25519.Verify(source.key.Public().(ed25519.PublicKey), []byte(parts[0]+"."+parts[1]+"tamper"), sig) {
		t.Fatal("tampered signature verified")
	}
	// The parsed key is reused even if the original file disappears.
	if err := os.Remove(cfg.PrivateKeyPath); err != nil {
		t.Fatal(err)
	}
	now = now.Add(800 * time.Second)
	second, _ := source.bearer()
	if second != first {
		t.Fatal("JWT refreshed unnecessarily")
	}
	now = now.Add(11 * time.Second)
	third, _ := source.bearer()
	if third == first || source.exp <= claims.Exp {
		t.Fatal("JWT did not refresh before expiry")
	}
	now = now.Add(20 * time.Minute)
	fourth, _ := source.bearer()
	if fourth == third || source.exp <= now.Unix() {
		t.Fatal("expired JWT reused")
	}
}

func TestJWTRejectsKeys(t *testing.T) {
	rsaKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	rsaDER, err := x509.MarshalPKCS8PrivateKey(rsaKey)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "bad.pem")
	for _, data := range [][]byte{[]byte("invalid"), pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: []byte("not private")}), pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: rsaDER}), append(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: []byte("invalid DER")}), []byte("extra")...)} {
		if err := os.WriteFile(path, data, 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := loadPrivateKey(path); err != errKey {
			t.Fatal("invalid key accepted or raw error exposed")
		}
	}
	if _, err := loadPrivateKey(path + "-missing"); err != errKey {
		t.Fatal("missing key accepted")
	}
}

func TestLocalDevelopmentPrivateKey(t *testing.T) {
	path := filepath.Join("..", "..", "..", "..", "data", "private", "qweather", "gofurry-dev-private.pem")
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Skip("local development key not installed")
	}
	key, err := loadPrivateKey(path)
	if err != nil {
		t.Fatal(err)
	}
	message := []byte("GoFurry development key local verification")
	if !ed25519.Verify(key.Public().(ed25519.PublicKey), message, ed25519.Sign(key, message)) {
		t.Fatal("development key verification failed")
	}
	t.Log("local PKCS#8 Ed25519 development key recognized; public-key verification passed")
}
