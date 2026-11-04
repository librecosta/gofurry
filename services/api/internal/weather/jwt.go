package weather

import (
	"crypto/ed25519"
	"crypto/x509"
	"encoding/base64"
	"encoding/json/v2"
	"encoding/pem"
	"errors"
	"io"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/librecosta/gofurry/services/api/internal/platform"
)

var errKey = errors.New("qweather requires a readable PKCS#8 Ed25519 private key")

func loadPrivateKey(path string) (ed25519.PrivateKey, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, errKey
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, 16<<10+1))
	if err != nil || len(data) > 16<<10 {
		return nil, errKey
	}
	block, rest := pem.Decode(data)
	if block == nil || block.Type != "PRIVATE KEY" || strings.TrimSpace(string(rest)) != "" {
		return nil, errKey
	}
	parsed, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, errKey
	}
	key, ok := parsed.(ed25519.PrivateKey)
	if !ok {
		return nil, errKey
	}
	return key, nil
}

type tokenSource struct {
	mu            sync.Mutex
	key           ed25519.PrivateKey
	kid, iss, sub string
	now           func() time.Time
	token         string
	iat, exp      int64
}

func newTokenSource(cfg platform.QWeatherConfig) (*tokenSource, error) {
	key, err := loadPrivateKey(cfg.PrivateKeyPath)
	if err != nil {
		return nil, err
	}
	return &tokenSource{key: key, kid: cfg.CredentialID, iss: cfg.DeveloperID, sub: cfg.ProjectID, now: time.Now}, nil
}

func (s *tokenSource) bearer() (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.now().Unix()
	if s.token != "" && now >= s.iat && now < s.exp-60 {
		return s.token, nil
	}
	iat := now - 30
	exp := iat + 900
	header, err := json.Marshal(struct {
		Alg string `json:"alg"`
		KID string `json:"kid"`
	}{"EdDSA", s.kid})
	if err != nil {
		return "", errKey
	}
	payload, err := json.Marshal(struct {
		Iss string `json:"iss"`
		Sub string `json:"sub"`
		Iat int64  `json:"iat"`
		Exp int64  `json:"exp"`
	}{s.iss, s.sub, iat, exp})
	if err != nil {
		return "", errKey
	}
	input := base64.RawURLEncoding.EncodeToString(header) + "." + base64.RawURLEncoding.EncodeToString(payload)
	signature := ed25519.Sign(s.key, []byte(input))
	if !ed25519.Verify(s.key.Public().(ed25519.PublicKey), []byte(input), signature) {
		return "", errKey
	}
	s.token = input + "." + base64.RawURLEncoding.EncodeToString(signature)
	s.iat = iat
	s.exp = exp
	return s.token, nil
}
