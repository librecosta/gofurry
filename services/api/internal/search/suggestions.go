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
	"github.com/redis/go-redis/v9"
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
	ErrDisabled    = errors.New("suggestions disabled")
)

type upstreamError struct {
	kind   string
	status int
}

func (*upstreamError) Error() string { return "suggestions upstream unavailable" }
func (*upstreamError) Unwrap() error { return ErrUnavailable }

// Only bounded categories/statuses are suitable for diagnostics, never raw
// transport errors (which contain query URLs and possibly proxy credentials).
func Diagnostic(err error) (string, int) {
	var upstream *upstreamError
	if errors.As(err, &upstream) {
		return upstream.kind, upstream.status
	}
	switch {
	case errors.Is(err, ErrDisabled):
		return "disabled", 0
	case errors.Is(err, ErrQuery):
		return "invalid_query", 0
	case errors.Is(err, ErrBusy):
		return "busy", 0
	case errors.Is(err, context.Canceled):
		return "cancelled", 0
	default:
		return "unavailable", 0
	}
}

type entry struct {
	phrases []string
	expires time.Time
}
type sharedEntry struct {
	Phrases []string  `json:"phrases"`
	Expires time.Time `json:"expires_at"`
}
type flight struct {
	done    chan struct{}
	phrases []string
	err     error
}

type Service struct {
	client    *http.Client
	transport *http.Transport
	enabled   bool
	configErr bool
	region    string
	redis     *redis.Client
	mu        sync.Mutex
	cache     map[string]entry
	flights   map[string]*flight
	ctx       context.Context
	cancel    context.CancelFunc
	closed    bool
}

func New(cfg *platform.SearchSuggestionsConfig, region string, shared *redis.Client) *Service {
	ctx, cancel := context.WithCancel(context.Background())
	s := &Service{enabled: cfg == nil || cfg.Enabled, region: region, redis: shared, cache: make(map[string]entry), flights: make(map[string]*flight), ctx: ctx, cancel: cancel}
	transport := &http.Transport{
		DialContext:         (&net.Dialer{Timeout: 2 * time.Second, KeepAlive: 30 * time.Second}).DialContext,
		TLSHandshakeTimeout: requestTimeout, ResponseHeaderTimeout: requestTimeout, MaxResponseHeaderBytes: 8 << 10,
		MaxConnsPerHost: 4, MaxIdleConnsPerHost: 4, IdleConnTimeout: 30 * time.Second,
	}
	if cfg != nil && cfg.ProxyURL != "" {
		proxy, err := url.Parse(cfg.ProxyURL)
		if err != nil || proxy.Hostname() == "" {
			s.configErr = true
		} else {
			transport.Proxy = http.ProxyURL(proxy)
		}
	}
	s.transport = transport
	s.client = &http.Client{Transport: transport, Timeout: requestTimeout, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	return s
}
func (s *Service) Close() {
	s.mu.Lock()
	s.closed = true
	s.mu.Unlock()
	s.cancel()
	s.transport.CloseIdleConnections()
}

func validText(text string) bool {
	return utf8.ValidString(text) && utf8.RuneCountInString(text) <= 128 && strings.IndexFunc(text, func(r rune) bool { return unicode.IsControl(r) || unicode.Is(unicode.Cf, r) }) < 0
}
func (s *Service) Suggestions(ctx context.Context, query string) ([]string, error) {
	if !validText(query) {
		return nil, ErrQuery
	}
	query = strings.TrimSpace(query)
	if query == "" {
		return []string{}, nil
	}
	if !s.enabled {
		return nil, ErrDisabled
	}
	if s.configErr {
		return nil, &upstreamError{kind: "configuration"}
	}
	key := platform.RedisKey(s.region, "suggestions", strings.ToLower(query))
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return nil, ErrUnavailable
	}
	if cached, ok := s.cache[key]; ok && time.Now().Before(cached.expires) {
		s.mu.Unlock()
		return append([]string{}, cached.phrases...), nil
	}
	pending := s.flights[key]
	if pending == nil {
		if len(s.flights) >= 4 {
			s.mu.Unlock()
			return nil, ErrBusy
		}
		pending = &flight{done: make(chan struct{})}
		s.flights[key] = pending
		go s.load(key, query, pending)
	}
	s.mu.Unlock()
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-pending.done:
		return append([]string{}, pending.phrases...), pending.err
	}
}

func (s *Service) load(key, query string, pending *flight) {
	ctx, cancel := context.WithTimeout(s.ctx, 4*time.Second)
	defer cancel()
	cached, err := s.sharedOrFetch(ctx, key, query)
	s.mu.Lock()
	defer s.mu.Unlock()
	if err == nil && !s.closed {
		if len(s.cache) >= maxCache {
			var oldest string
			var expires time.Time
			for k, v := range s.cache {
				if expires.IsZero() || v.expires.Before(expires) {
					oldest, expires = k, v.expires
				}
			}
			delete(s.cache, oldest)
		}
		s.cache[key] = entry{phrases: append([]string{}, cached.phrases...), expires: cached.expires}
	}
	pending.phrases, pending.err = cached.phrases, err
	delete(s.flights, key)
	close(pending.done)
}
func (s *Service) sharedOrFetch(ctx context.Context, key, query string) (entry, error) {
	read := func() (entry, bool, bool) {
		raw, hit, available := platform.RedisRead(ctx, s.redis, key)
		var value sharedEntry
		if hit && len(raw) <= maxBody && json.Unmarshal(raw, &value) == nil && value.Phrases != nil && len(value.Phrases) <= maxSuggestions && time.Now().Before(value.Expires) && value.Expires.Sub(time.Now()) <= cacheTTL {
			for _, p := range value.Phrases {
				if !validText(p) || strings.TrimSpace(p) == "" {
					return entry{}, false, available
				}
			}
			return entry{phrases: value.Phrases, expires: value.Expires}, true, available
		}
		return entry{}, false, available
	}
	cached, hit, available := read()
	if hit {
		return cached, nil
	}
	if available {
		release, owned, reachable := platform.RedisLease(ctx, s.redis, key, 5*time.Second)
		defer release()
		if reachable && !owned {
			ticker := time.NewTicker(50 * time.Millisecond)
			defer ticker.Stop()
			for {
				select {
				case <-ctx.Done():
					return entry{}, ErrBusy
				case <-ticker.C:
					cached, hit, available = read()
					if hit {
						return cached, nil
					}
					if !available {
						phrases, err := s.fetch(ctx, query)
						return entry{phrases: phrases, expires: time.Now().Add(cacheTTL)}, err
					}
				}
			}
		}
		available = reachable
		// The owner may have finished between the first GET and our lease.
		if available {
			if cached, hit, _ = read(); hit {
				return cached, nil
			}
		}
	}
	phrases, err := s.fetch(ctx, query)
	cached = entry{phrases: phrases, expires: time.Now().Add(cacheTTL)}
	if err == nil && available {
		data, _ := json.Marshal(sharedEntry{Phrases: phrases, Expires: cached.expires})
		platform.RedisWrite(ctx, s.redis, key, platform.RedisKey(s.region, "suggestions", "index"), data, cacheTTL, maxCache)
	}
	return cached, err
}
func (s *Service) fetch(ctx context.Context, query string) ([]string, error) {
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, "https://duckduckgo.com/ac/?"+url.Values{"q": {query}}.Encode(), nil)
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; GoFurry/0.1)")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Accept-Language", "zh-CN,zh;q=0.9,en;q=0.8")
	response, err := s.client.Do(req)
	if err != nil {
		kind := "network"
		var timeout net.Error
		if errors.As(err, &timeout) && timeout.Timeout() {
			kind = "timeout"
		}
		return nil, &upstreamError{kind: kind}
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, &upstreamError{kind: "upstream_http", status: response.StatusCode}
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, maxBody+1))
	if err != nil {
		return nil, &upstreamError{kind: "response_body"}
	}
	if len(body) > maxBody {
		return nil, &upstreamError{kind: "response_size"}
	}
	var decoded []struct {
		Phrase *string `json:"phrase"`
	}
	if err = json.Unmarshal(body, &decoded); err != nil || decoded == nil {
		return nil, &upstreamError{kind: "invalid_response"}
	}
	phrases := []string{}
	seen := make(map[string]bool)
	for _, item := range decoded {
		if item.Phrase == nil {
			return nil, &upstreamError{kind: "invalid_response"}
		}
		if !validText(*item.Phrase) {
			continue
		}
		phrase := strings.TrimSpace(*item.Phrase)
		id := strings.ToLower(phrase)
		if phrase == "" || seen[id] {
			continue
		}
		seen[id] = true
		phrases = append(phrases, phrase)
		if len(phrases) == maxSuggestions {
			break
		}
	}
	return phrases, nil
}
