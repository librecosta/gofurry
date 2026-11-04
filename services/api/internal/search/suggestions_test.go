package search

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/librecosta/gofurry/services/api/internal/platform"
)

type roundTrip func(*http.Request) (*http.Response, error)

func (f roundTrip) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func reply(status int, body string) *http.Response {
	return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(body))}
}

func TestSuggestionsJSONv2AndBounds(t *testing.T) {
	s := New(&platform.SearchSuggestionsConfig{Enabled: true}, "INTL", nil)
	defer s.Close()
	for _, tc := range []struct {
		name, body    string
		status, count int
		failed        bool
	}{
		{"dedup without content restrictions", `[{"phrase":" Go tutorial "},{"phrase":"go tutorial"},{"phrase":"porn"},{"phrase":"ｐｏｒｎ"},{"phrase":"safe"}]`, 200, 4, false},
		{"empty", `[]`, 200, 0, false},
		{"null", `null`, 200, 0, true},
		{"missing phrase", `[{}]`, 200, 0, true},
		{"wrong type", `[{"phrase":42}]`, 200, 0, true},
		{"invalid JSON", `[`, 200, 0, true},
		{"duplicate field", `[{"phrase":"a","phrase":"b"}]`, 200, 0, true},
		{"oversize", strings.Repeat(" ", maxBody+1), 200, 0, true},
		{"redirect", `[]`, 302, 0, true},
		{"upstream 429", `[]`, 429, 0, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s.cache = make(map[string]entry)
			s.client.Transport = roundTrip(func(r *http.Request) (*http.Response, error) {
				if r.URL.Host != "duckduckgo.com" || r.URL.Path != "/ac/" || r.URL.Query().Get("q") != "Go" || r.Header.Get("Accept") != "application/json" {
					t.Error("incorrect official request")
				}
				if _, ok := r.Context().Deadline(); !ok {
					t.Error("missing deadline")
				}
				return reply(tc.status, tc.body), nil
			})
			got, err := s.Suggestions(context.Background(), " Go ")
			if (err != nil) != tc.failed || len(got) != tc.count {
				t.Fatalf("got %v, err %v", got, err)
			}
		})
	}
	if _, err := s.Suggestions(context.Background(), strings.Repeat("中", 129)); err != ErrQuery {
		t.Fatal("unbounded query")
	}
}

func TestSuggestionsCacheLimitsAndCopies(t *testing.T) {
	s := New(&platform.SearchSuggestionsConfig{Enabled: true}, "INTL", nil)
	defer s.Close()
	var calls int
	s.client.Transport = roundTrip(func(r *http.Request) (*http.Response, error) {
		calls++
		return reply(200, `[{"phrase":"a"},{"phrase":"b"},{"phrase":"c"},{"phrase":"d"},{"phrase":"e"},{"phrase":"f"},{"phrase":"g"},{"phrase":"h"},{"phrase":"i"}]`), nil
	})
	a, _ := s.Suggestions(context.Background(), "go")
	a[0] = "mutated"
	b, _ := s.Suggestions(context.Background(), "GO")
	if calls != 1 || b[0] != "a" || len(b) != maxSuggestions {
		t.Fatal("cache is mutable or unbounded")
	}
	s.cache[platform.RedisKey("INTL", "suggestions", "go")] = entry{expires: time.Now().Add(-time.Second)}
	s.Suggestions(context.Background(), "go")
	if calls != 2 {
		t.Fatal("expired entry reused")
	}
	for i := range maxCache + 1 {
		s.Suggestions(context.Background(), strings.Repeat("a", i/128+1)+string(rune(0x4e00+i)))
	}
	if len(s.cache) > maxCache {
		t.Fatal("unbounded cache")
	}
}

func TestSuggestionsRegionalEqualityAndExplicitProxy(t *testing.T) {
	t.Setenv("HTTP_PROXY", "http://127.0.0.1:1")
	t.Setenv("HTTPS_PROXY", "http://127.0.0.1:1")
	for _, region := range []string{"CN", "INTL"} {
		for _, cfg := range []*platform.SearchSuggestionsConfig{nil, {Enabled: true}, {Enabled: true, ProxyURL: "http://127.0.0.1:7897"}} {
			s := New(cfg, region, nil)
			if cfg == nil || cfg.ProxyURL == "" {
				if s.transport.Proxy != nil {
					t.Fatal("environment proxy used")
				}
			} else {
				proxy, _ := s.transport.Proxy(&http.Request{})
				if proxy.String() != cfg.ProxyURL {
					t.Fatal("explicit proxy ignored")
				}
			}
			s.client.Transport = roundTrip(func(*http.Request) (*http.Response, error) {
				return reply(200, `[{"phrase":"furry"},{"phrase":"兽人"},{"phrase":"GoFurry"},{"phrase":"unreviewed phrase"},{"phrase":"porn"}]`), nil
			})
			for _, q := range []string{"furry", "兽人", "GoFurry", "p.o.r.n"} {
				got, err := s.Suggestions(context.Background(), q)
				if err != nil || len(got) != 5 {
					t.Fatal("regional/content restriction remains", region, q)
				}
			}
			s.Close()
		}
		disabled := New(&platform.SearchSuggestionsConfig{Enabled: false}, region, nil)
		if _, err := disabled.Suggestions(context.Background(), "furry"); !errors.Is(err, ErrDisabled) {
			t.Fatal("explicit disable is not diagnosable")
		}
		disabled.Close()
	}
}

func TestSuggestionsFailureDiagnosticsAndInput(t *testing.T) {
	for _, tc := range []struct {
		status     int
		body, kind string
	}{
		{503, `[]`, "upstream_http"}, {429, `[]`, "upstream_http"}, {200, `not json`, "invalid_response"}, {200, strings.Repeat("x", maxBody+1), "response_size"},
	} {
		s := New(nil, "CN", nil)
		s.client.Transport = roundTrip(func(*http.Request) (*http.Response, error) { return reply(tc.status, tc.body), nil })
		_, err := s.Suggestions(context.Background(), "furry")
		kind, status := Diagnostic(err)
		if err == nil || kind != tc.kind || (tc.kind == "upstream_http" && status != tc.status) {
			t.Fatal("upstream error was disguised as empty success")
		}
		s.Close()
	}
	s := New(nil, "CN", nil)
	defer s.Close()
	s.client.Transport = roundTrip(func(*http.Request) (*http.Response, error) { return nil, context.DeadlineExceeded })
	_, err := s.Suggestions(context.Background(), "furry")
	if kind, _ := Diagnostic(err); kind != "timeout" {
		t.Fatal("timeout not diagnosed")
	}
	for _, q := range []string{"go\nlang", "go\u200blang", "\xff", strings.Repeat("城", 129)} {
		if _, err := s.Suggestions(context.Background(), q); !errors.Is(err, ErrQuery) {
			t.Fatal("invalid input accepted")
		}
	}
}

func TestSuggestionsSameKeyMerging(t *testing.T) {
	s := New(nil, "CN", nil)
	defer s.Close()
	var calls atomic.Int32
	entered, release := make(chan struct{}), make(chan struct{})
	s.client.Transport = roundTrip(func(*http.Request) (*http.Response, error) {
		if calls.Add(1) == 1 {
			close(entered)
		}
		<-release
		return reply(200, `[{"phrase":"furry"}]`), nil
	})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { _, err := s.Suggestions(ctx, "furry"); done <- err }()
	<-entered
	remaining := make(chan error, 1)
	go func() { _, err := s.Suggestions(context.Background(), "furry"); remaining <- err }()
	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatal("waiter cancellation ignored")
	}
	close(release)
	if err := <-remaining; err != nil || calls.Load() != 1 {
		t.Fatal("same key was not merged")
	}
}

func TestSuggestionsConcurrentAdmissionAndCancellation(t *testing.T) {
	s := New(&platform.SearchSuggestionsConfig{Enabled: true}, "INTL", nil)
	defer s.Close()
	var started atomic.Int32
	s.client.Transport = roundTrip(func(r *http.Request) (*http.Response, error) {
		started.Add(1)
		<-r.Context().Done()
		return nil, r.Context().Err()
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan struct{}, 4)
	for i := range 4 {
		go func() { s.Suggestions(ctx, "go"+strconv.Itoa(i)); done <- struct{}{} }()
	}
	deadline := time.Now().Add(time.Second)
	for started.Load() != 4 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if started.Load() != 4 {
		t.Fatal("requests did not start")
	}
	if _, err := s.Suggestions(ctx, "another"); err != ErrBusy {
		t.Fatal("unbounded upstream concurrency")
	}
	cancel()
	for range 4 {
		select {
		case <-done:
		case <-time.After(time.Second):
			t.Fatal("cancellation ignored")
		}
	}
}
