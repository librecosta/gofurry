package catalog

import (
	"crypto/rand"
	"encoding/base32"
	"errors"
	"net"
	"net/netip"
	"net/url"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"golang.org/x/net/idna"
)

var (
	ErrInvalid     = errors.New("invalid catalog input")
	ErrNotFound    = errors.New("catalog entity not found")
	ErrConflict    = errors.New("catalog state, revision or source conflict")
	ErrUnavailable = errors.New("catalog unavailable")

	publicEncoding = base32.NewEncoding("0123456789abcdefghjkmnpqrstvwxyz").WithPadding(base32.NoPadding)
	publicPattern  = regexp.MustCompile(`^[0123456789abcdefghjkmnpqrstvwxyz]{16}$`)
	slugPattern    = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)
	latinLabel     = regexp.MustCompile(`^[A-Za-z0-9 _-]+$`)
	platformCode   = regexp.MustCompile(`^[a-z][a-z0-9_]{0,31}$`)
	dnsLabel       = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?$`)
	decimal        = regexp.MustCompile(`^[0-9]+$`)
)

const (
	Draft     = "draft"
	Published = "published"
	Archived  = "archived"
)

type Localization struct {
	Locale  string `json:"locale"`
	Name    string `json:"name"`
	Summary string `json:"summary,omitempty"`
}

type Address struct {
	Kind string `json:"kind"`
	URL  string `json:"url"`
}

type Source struct {
	PlatformCode string `json:"platform_code"`
	ExternalKey  string `json:"external_key"`
	SourceURL    string `json:"source_url,omitempty"`
}

type Editorial struct {
	Slug          string
	Localizations []Localization
}

type SiteInput struct {
	Editorial
	Addresses []Address
}

type GameInput struct {
	Editorial
	Sources []Source
}

func newPublicID() string {
	var entropy [10]byte
	rand.Read(entropy[:])
	return publicEncoding.EncodeToString(entropy[:])
}

func ValidPublicID(value string) bool { return publicPattern.MatchString(value) }

func normalizeEditorial(in Editorial, previousSlug string) (Editorial, error) {
	if len(in.Localizations) > 2 {
		return Editorial{}, ErrInvalid
	}
	in.Localizations = slices.Clone(in.Localizations)
	seen := map[string]bool{}
	for i, l := range in.Localizations {
		l.Name, l.Summary = strings.TrimSpace(l.Name), strings.TrimSpace(l.Summary)
		if (l.Locale != "en" && l.Locale != "zh") || seen[l.Locale] ||
			!validText(l.Name, 1, 160, false) || !validText(l.Summary, 0, 2000, true) {
			return Editorial{}, ErrInvalid
		}
		seen[l.Locale] = true
		in.Localizations[i] = l
	}

	slug := strings.TrimSpace(in.Slug)
	if slug == "" {
		slug = previousSlug
	}
	if slug == "" {
		for _, locale := range []string{"en", "zh"} {
			for _, text := range in.Localizations {
				if text.Locale != locale || !latinLabel.MatchString(text.Name) {
					continue
				}
				words := strings.FieldsFunc(text.Name, func(r rune) bool { return r == ' ' || r == '-' || r == '_' })
				candidate := strings.ToLower(strings.Join(words, "-"))
				if len(candidate) <= 80 && slugPattern.MatchString(candidate) {
					in.Slug = candidate
					return in, nil
				}
			}
		}
	}
	if !latinLabel.MatchString(slug) {
		return Editorial{}, ErrInvalid
	}
	in.Slug = strings.ToLower(slug)
	if len(in.Slug) > 80 || !slugPattern.MatchString(in.Slug) {
		return Editorial{}, ErrInvalid
	}
	return in, nil
}

func validText(s string, min, max int, multiline bool) bool {
	if !utf8.ValidString(s) {
		return false
	}
	if count := utf8.RuneCountInString(s); count < min || count > max {
		return false
	}
	for _, r := range s {
		if unicode.IsControl(r) && !(multiline && r == '\n') {
			return false
		}
	}
	return true
}

func normalizeAddresses(in []Address) ([]Address, error) {
	if len(in) > 16 {
		return nil, ErrInvalid
	}
	out := make([]Address, 0, len(in))
	seen := map[string]bool{}
	primary := false
	for _, a := range in {
		address, err := normalizeURL(a.URL)
		if err != nil || seen[address] || (a.Kind != "primary" && a.Kind != "alternate") || (a.Kind == "primary" && primary) {
			return nil, ErrInvalid
		}
		primary = primary || a.Kind == "primary"
		seen[address] = true
		out = append(out, Address{Kind: a.Kind, URL: address})
	}
	return out, nil
}

func normalizeSources(in []Source) ([]Source, error) {
	if len(in) > 16 {
		return nil, ErrInvalid
	}
	out := make([]Source, 0, len(in))
	seen := map[[2]string]bool{}
	for _, s := range in {
		s.PlatformCode = strings.ToLower(strings.TrimSpace(s.PlatformCode))
		s.ExternalKey = strings.TrimSpace(s.ExternalKey)
		if !platformCode.MatchString(s.PlatformCode) || !validText(s.ExternalKey, 1, 160, false) {
			return nil, ErrInvalid
		}
		if s.SourceURL != "" {
			var err error
			s.SourceURL, err = normalizeURL(s.SourceURL)
			if err != nil {
				return nil, err
			}
		}
		if s.PlatformCode == "steam" {
			appID, err := strconv.ParseUint(s.ExternalKey, 10, 32)
			if err != nil || appID == 0 || !decimal.MatchString(s.ExternalKey) {
				return nil, ErrInvalid
			}
			s.ExternalKey = strconv.FormatUint(appID, 10)
			steamURL := "https://store.steampowered.com/app/" + s.ExternalKey
			if s.SourceURL != "" && s.SourceURL != steamURL && s.SourceURL != steamURL+"/" {
				return nil, ErrInvalid
			}
			s.SourceURL = steamURL + "/"
		}
		key := [2]string{s.PlatformCode, s.ExternalKey}
		if seen[key] {
			return nil, ErrInvalid
		}
		seen[key] = true
		out = append(out, s)
	}
	slices.SortFunc(out, func(a, b Source) int {
		if byPlatform := strings.Compare(a.PlatformCode, b.PlatformCode); byPlatform != 0 {
			return byPlatform
		}
		return strings.Compare(a.ExternalKey, b.ExternalKey)
	})
	return out, nil
}

func normalizeURL(raw string) (string, error) {
	if !validText(raw, 1, 2048, false) || strings.ContainsAny(raw, "#\\") || strings.ContainsFunc(raw, unicode.IsSpace) {
		return "", ErrInvalid
	}
	u, err := url.Parse(raw)
	if err != nil || u.User != nil || u.Hostname() == "" || (u.Scheme != "http" && u.Scheme != "https") ||
		!validText(u.Path, 0, 2048, false) || strings.Contains(u.Path, "\\") {
		return "", ErrInvalid
	}
	// Validate decoded query components without re-encoding them or changing
	// their order, duplicate keys, escaping or use of empty values.
	for pair := range strings.FieldsFuncSeq(u.RawQuery, func(r rune) bool { return r == '&' || r == ';' }) {
		rawName, rawValue, _ := strings.Cut(pair, "=")
		name, nameErr := url.QueryUnescape(rawName)
		value, valueErr := url.QueryUnescape(rawValue)
		if nameErr != nil || valueErr != nil || !validText(name, 0, 2048, false) ||
			!validText(value, 0, 2048, false) || credentialKey(name) {
			return "", ErrInvalid
		}
	}

	host := strings.TrimSuffix(strings.ToLower(u.Hostname()), ".")
	port := u.Port()
	if strings.HasSuffix(u.Host, ":") {
		return "", ErrInvalid
	}
	if port != "" {
		n, err := strconv.ParseUint(port, 10, 16)
		if err != nil || n == 0 {
			return "", ErrInvalid
		}
		port = strconv.FormatUint(n, 10)
		if (u.Scheme == "https" && n == 443) || (u.Scheme == "http" && n == 80) {
			port = ""
		}
	}
	if addr, err := netip.ParseAddr(host); err == nil && addr.Zone() == "" {
		host = addr.String()
		if port == "" && addr.Is6() {
			host = "[" + host + "]"
		}
	} else {
		if strings.ContainsAny(u.Host, "[]") {
			return "", ErrInvalid
		}
		host, err = idna.Lookup.ToASCII(host)
		if err != nil || len(host) > 253 {
			return "", ErrInvalid
		}
		host = strings.ToLower(host)
		for label := range strings.SplitSeq(host, ".") {
			if !dnsLabel.MatchString(label) {
				return "", ErrInvalid
			}
		}
	}
	u.Host = host
	if port != "" {
		u.Host = net.JoinHostPort(host, port)
	}
	canonical := u.String()
	if len(canonical) > 2048 {
		return "", ErrInvalid
	}
	return canonical, nil
}

func credentialKey(key string) bool {
	key = strings.Map(func(r rune) rune {
		if r == '-' || r == '_' || unicode.IsSpace(r) {
			return -1
		}
		return unicode.ToLower(r)
	}, key)
	if strings.HasPrefix(key, "xamz") || strings.HasPrefix(key, "xgoog") {
		return true
	}
	switch key {
	case "password", "passwd", "pwd", "secret", "clientsecret", "authorization", "auth", "credential", "credentials",
		"token", "accesstoken", "idtoken", "refreshtoken", "authtoken", "apikey", "signature", "sig", "sessionid":
		return true
	default:
		return false
	}
}
