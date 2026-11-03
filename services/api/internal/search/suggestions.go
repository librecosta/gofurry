package search

import (
	"context"
	"encoding/json/v2"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/librecosta/gofurry/services/api/internal/platform"
)

const (
	maxBody        = 64 << 10
	maxSuggestions = 8
	maxCache       = 256
	requestTimeout = 3 * time.Second
	cacheTTL       = 10 * time.Minute
)

var (
	ErrQuery       = errors.New("invalid suggestion query")
	ErrUnavailable = errors.New("suggestions unavailable")
	ErrBusy        = errors.New("suggestions busy")
)

type entry struct {
	phrases []string
	expires time.Time
}

// Service owns one regional upstream client and a bounded, public-query cache.
// There is no legacy service, user data, environment proxy or credential fallback.
type Service struct {
	client    *http.Client
	transport *http.Transport
	enabled   bool
	region    string
	allow     map[string]bool
	mu        sync.Mutex
	cache     map[string]entry
	slots     chan struct{}
}

func New(cfg *platform.SearchSuggestionsConfig, region string) *Service {
	s := &Service{region: region, allow: make(map[string]bool), cache: make(map[string]entry), slots: make(chan struct{}, 4)}
	transport := &http.Transport{
		DialContext:         (&net.Dialer{Timeout: 2 * time.Second, KeepAlive: 30 * time.Second}).DialContext,
		TLSHandshakeTimeout: 2 * time.Second, ResponseHeaderTimeout: requestTimeout,
		MaxResponseHeaderBytes: 8 << 10, MaxConnsPerHost: 4, MaxIdleConnsPerHost: 4, IdleConnTimeout: 30 * time.Second,
	}
	if cfg != nil {
		s.enabled = cfg.Enabled && (region == "CN" || region == "INTL")
		for _, phrase := range cfg.CNAllowlist {
			phrase = strings.TrimSpace(phrase)
			if safe(phrase) {
				s.allow[strings.ToLower(phrase)] = true
			}
		}
		if cfg.ProxyURL != "" {
			proxy, err := url.Parse(cfg.ProxyURL)
			if err != nil {
				s.enabled = false
			} else {
				transport.Proxy = http.ProxyURL(proxy)
			}
		}
	}
	if region == "CN" && len(s.allow) == 0 {
		s.enabled = false
	}
	s.transport = transport
	s.client = &http.Client{Transport: transport, Timeout: requestTimeout, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	return s
}

func (s *Service) Close() { s.transport.CloseIdleConnections() }

func (s *Service) Suggestions(ctx context.Context, query string) ([]string, error) {
	query = strings.TrimSpace(query)
	if !utf8.ValidString(query) || utf8.RuneCountInString(query) > 128 {
		return nil, ErrQuery
	}
	empty := []string{}
	if !s.enabled || query == "" || !safe(query) {
		return empty, nil
	}
	key := strings.ToLower(query)
	s.mu.Lock()
	cached, ok := s.cache[key]
	s.mu.Unlock()
	if ok && time.Now().Before(cached.expires) {
		return append(empty, cached.phrases...), nil
	}
	select {
	case s.slots <- struct{}{}:
		defer func() { <-s.slots }()
	default:
		return nil, ErrBusy
	}
	ctx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://duckduckgo.com/ac/?"+url.Values{"q": {query}}.Encode(), nil)
	if err != nil {
		return nil, ErrUnavailable
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; GoFurry/0.1)")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Accept-Language", "zh-CN,zh;q=0.9,en;q=0.8")
	response, err := s.client.Do(req)
	if err != nil {
		return nil, ErrUnavailable
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, ErrUnavailable
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, maxBody+1))
	if err != nil || len(body) > maxBody {
		return nil, ErrUnavailable
	}
	var decoded []struct {
		Phrase *string `json:"phrase"`
	}
	if err = json.Unmarshal(body, &decoded); err != nil || decoded == nil {
		return nil, ErrUnavailable
	}
	phrases := empty
	seen := make(map[string]bool)
	for _, item := range decoded {
		if item.Phrase == nil {
			return nil, ErrUnavailable
		}
		phrase := strings.TrimSpace(*item.Phrase)
		id := strings.ToLower(phrase)
		if utf8.RuneCountInString(phrase) > 128 || !safe(phrase) || seen[id] || (s.region == "CN" && !s.allow[id]) {
			continue
		}
		seen[id] = true
		phrases = append(phrases, phrase)
		if len(phrases) == maxSuggestions {
			break
		}
	}
	s.mu.Lock()
	if len(s.cache) >= maxCache {
		var oldest string
		var expires time.Time
		for k, value := range s.cache {
			if expires.IsZero() || value.expires.Before(expires) {
				oldest, expires = k, value.expires
			}
		}
		delete(s.cache, oldest)
	}
	s.cache[key] = entry{phrases: append(empty, phrases...), expires: time.Now().Add(cacheTTL)}
	s.mu.Unlock()
	return phrases, nil
}

// A conservative denylist is defense in depth, not an assertion that third-party
// suggestions are moderated. CN additionally requires exact reviewed matches.
func safe(phrase string) bool {
	if phrase == "" {
		return false
	}
	var folded strings.Builder
	for _, r := range strings.ToLower(phrase) {
		if unicode.IsControl(r) || unicode.Is(unicode.Cf, r) {
			return false
		}
		if r >= '！' && r <= '～' {
			r -= 0xfee0
		}
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			folded.WriteRune(r)
		}
	}
	text := folded.String()
	for _, blocked := range []string{"porn", "nsfw", "nude", "naked", "sex", "xxx", "hentai", "yiff", "fetish", "rule34", "r34", "xvideos", "色情", "成人", "裸体", "裸聊", "做爱", "性爱", "性交", "黄片", "色图", "兽交"} {
		if strings.Contains(text, blocked) {
			return false
		}
	}
	return text != ""
}
