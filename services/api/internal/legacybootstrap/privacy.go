package legacybootstrap

import (
	"bytes"
	"context"
	"encoding/json/jsontext"
	"errors"
	"io"
	"net/url"
	"regexp"
	"strings"

	"github.com/librecosta/gofurry/services/api/internal/legacyseed"
)

var (
	credentialText = regexp.MustCompile(`(?i)-----BEGIN [A-Z ]*PRIVATE KEY-----|\bbearer\s+[a-z0-9._-]{12,}|\b(?:postgres(?:ql)?|rediss?|mongodb(?:\+srv)?)://|\b(?:password|access[_-]?token|api[_-]?key|client[_-]?secret)\s*[:=]\s*[^\s]{6}`)
	webURL         = regexp.MustCompile(`(?i)https?://[^\s"'<>\\]+`)
	credentialKey  = regexp.MustCompile(`(?i)^(?:token|password|authorization|secret|secret_access_key|client_secret|api[-_]?key|access_token|refresh_token|session_token|cookie|signature|x-amz-.*)$`)
	errSensitive   = errors.New("bootstrap material contains a possible credential")
)

func screen(ctx context.Context, seed *legacyseed.Bundle, decisions []byte) error {
	if err := scanJSON(decisions); err != nil {
		return err
	}
	return seed.Content(ctx, func(row legacyseed.Record) error {
		for _, value := range row.Raw {
			if err := scanJSON(value); err != nil {
				return err
			}
		}
		return scanJSON(row.Candidate)
	})
}

func scanJSON(raw []byte) error {
	d := jsontext.NewDecoder(bytes.NewReader(raw))
	for {
		container, position := d.StackIndex(d.StackDepth())
		token, err := d.ReadToken()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return errors.New("invalid bootstrap JSON")
		}
		if token.Kind() == '"' && (sensitive(token.String()) ||
			container == '{' && position%2 == 0 && credentialKey.MatchString(token.String())) {
			return errSensitive
		}
	}
}

func sensitive(value string) bool {
	if credentialText.MatchString(value) {
		return true
	}
	for _, link := range webURL.FindAllString(value, -1) {
		u, err := url.Parse(link)
		if err != nil {
			continue
		}
		if u.User != nil {
			return true
		}
		for k := range u.Query() {
			if credentialKey.MatchString(k) || strings.EqualFold(k, "sig") || strings.EqualFold(k, "session") {
				return true
			}
		}
	}
	return false
}
