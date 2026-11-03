package search

import (
	"context"
	"io"
	"net/http"
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
	s := New(&platform.SearchSuggestionsConfig{Enabled: true}, "INTL")
	defer s.Close()
	for _, tc := range []struct {
		name, body    string
		status, count int
		failed        bool
	}{
		{"dedup and SFW", `[{"phrase":" Go tutorial "},{"phrase":"go tutorial"},{"phrase":"porn"},{"phrase":"ｐｏｒｎ"},{"phrase":"safe"}]`, 200, 2, false},
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
	s := New(&platform.SearchSuggestionsConfig{Enabled: true}, "INTL")
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
	s.cache["go"] = entry{expires: time.Now().Add(-time.Second)}
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

func TestSuggestionsCNFailClosedAndExplicitProxy(t *testing.T) {
	t.Setenv("HTTP_PROXY", "http://127.0.0.1:1")
	t.Setenv("HTTPS_PROXY", "http://127.0.0.1:1")
	for _, cfg := range []*platform.SearchSuggestionsConfig{nil, {Enabled: false}, {Enabled: true}} {
		s := New(cfg, "CN")
		s.client.Transport = roundTrip(func(*http.Request) (*http.Response, error) { t.Fatal("disabled CN called upstream"); return nil, nil })
		got, err := s.Suggestions(context.Background(), "go")
		if err != nil || len(got) != 0 || s.transport.Proxy != nil {
			t.Fatal("CN/environment fail-closed broken")
		}
		s.Close()
	}
	s := New(&platform.SearchSuggestionsConfig{Enabled: true, ProxyURL: "http://127.0.0.1:7897", CNAllowlist: []string{"Go tutorial", "porn"}}, "CN")
	defer s.Close()
	proxy, _ := s.transport.Proxy(&http.Request{})
	if proxy.String() != "http://127.0.0.1:7897" {
		t.Fatal("YAML proxy ignored")
	}
	s.client.Transport = roundTrip(func(*http.Request) (*http.Response, error) {
		return reply(200, `[{"phrase":"Go tutorial"},{"phrase":"unreviewed phrase"},{"phrase":"porn"}]`), nil
	})
	got, err := s.Suggestions(context.Background(), "go")
	if err != nil || len(got) != 1 || got[0] != "Go tutorial" {
		t.Fatalf("CN leaked unreviewed suggestions: %v %v", got, err)
	}
	if got, _ := s.Suggestions(context.Background(), "p.o.r.n"); len(got) != 0 {
		t.Fatal("blocked query accepted")
	}
}

func TestSuggestionsConcurrentAdmissionAndCancellation(t *testing.T) {
	s := New(&platform.SearchSuggestionsConfig{Enabled: true}, "INTL")
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
	for range 4 {
		go func() { s.Suggestions(ctx, "go"); done <- struct{}{} }()
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
