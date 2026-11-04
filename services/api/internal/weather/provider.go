package weather

import (
	"context"
	"encoding/json/v2"
	"errors"
	"fmt"
	"io"
	"math"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/librecosta/gofurry/services/api/internal/platform"
)

const requestTimeout = 4 * time.Second
const maxBody = 128 << 10
const maxConcurrent = 4

var ErrUnavailable = errors.New("weather unavailable")
var ErrConfig = errors.New("qweather configuration incomplete")

type Current struct {
	TemperatureC  float64   `json:"temperature_c"`
	ConditionCode string    `json:"condition_code"`
	ConditionText string    `json:"condition_text"`
	FetchedAt     time.Time `json:"fetched_at"`
}

type observation struct {
	current      Current
	attributions []string
}
type upstreamError struct {
	status   int
	cooldown time.Duration
}

func (*upstreamError) Error() string { return "weather upstream unavailable" }

type Provider struct {
	baseURL   string
	tokens    *tokenSource
	client    *http.Client
	transport *http.Transport
	slots     chan struct{}
	ctx       context.Context
	cancel    context.CancelFunc
	now       func() time.Time
}

func NewProvider(cfg *platform.QWeatherConfig) (*Provider, error) {
	if cfg == nil || !cfg.Enabled {
		return nil, nil
	}
	if len(cfg.MissingFields()) != 0 {
		return nil, ErrConfig
	}
	if err := cfg.Validate(); err != nil {
		return nil, ErrConfig
	}
	tokens, err := newTokenSource(*cfg)
	if err != nil {
		return nil, err
	}
	transport := &http.Transport{DialContext: (&net.Dialer{Timeout: 2 * time.Second, KeepAlive: 30 * time.Second}).DialContext,
		TLSHandshakeTimeout: 2 * time.Second, ResponseHeaderTimeout: requestTimeout, MaxResponseHeaderBytes: 16 << 10,
		MaxConnsPerHost: maxConcurrent, MaxIdleConns: maxConcurrent, MaxIdleConnsPerHost: maxConcurrent, IdleConnTimeout: 60 * time.Second}
	if cfg.ProxyURL != "" {
		proxy, err := url.Parse(cfg.ProxyURL)
		if err != nil {
			return nil, ErrConfig
		}
		transport.Proxy = http.ProxyURL(proxy)
	}
	ctx, cancel := context.WithCancel(context.Background())
	return &Provider{baseURL: "https://" + strings.ToLower(cfg.APIHost), tokens: tokens, transport: transport,
		client: &http.Client{Transport: transport, Timeout: requestTimeout, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }},
		slots:  make(chan struct{}, maxConcurrent), ctx: ctx, cancel: cancel, now: time.Now}, nil
}

func (p *Provider) Close() {
	if p != nil {
		p.cancel()
		p.transport.CloseIdleConnections()
	}
}

func normalized(lat, lon float64, lang string) (float64, float64, string, bool) {
	if (lang != "zh" && lang != "en") || math.IsNaN(lat) || math.IsNaN(lon) || math.IsInf(lat, 0) || math.IsInf(lon, 0) || lat < -90 || lat > 90 || lon < -180 || lon > 180 {
		return 0, 0, "", false
	}
	lat = math.Round(lat*100) / 100
	lon = math.Round(lon*100) / 100
	if lat == 0 {
		lat = 0
	}
	if lon == 0 {
		lon = 0
	}
	return lat, lon, fmt.Sprintf("%.2f/%.2f/%s", lat, lon, lang), true
}

func (p *Provider) Fetch(ctx context.Context, lat, lon float64, lang string) (*observation, error) {
	lat, lon, _, valid := normalized(lat, lon, lang)
	if p == nil || !valid || p.ctx.Err() != nil {
		return nil, ErrUnavailable
	}
	select {
	case p.slots <- struct{}{}:
		defer func() { <-p.slots }()
	default:
		return nil, ErrUnavailable
	}
	ctx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()
	stop := context.AfterFunc(p.ctx, cancel)
	defer stop()
	token, err := p.tokens.bearer()
	if err != nil {
		return nil, ErrUnavailable
	}
	endpoint := fmt.Sprintf("%s/weather/v1/current/%.2f/%.2f?lang=%s", p.baseURL, lat, lon, lang)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, ErrUnavailable
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")
	res, err := p.client.Do(req)
	if err != nil {
		return nil, ErrUnavailable
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		cooldown := 15 * time.Second
		if res.StatusCode == 429 {
			cooldown = time.Minute
			if seconds, err := strconv.Atoi(res.Header.Get("Retry-After")); err == nil && seconds > 60 {
				cooldown = time.Duration(min(seconds, 300)) * time.Second
			}
		}
		return nil, &upstreamError{status: res.StatusCode, cooldown: cooldown}
	}
	body, err := io.ReadAll(io.LimitReader(res.Body, maxBody+1))
	if err != nil || len(body) > maxBody {
		return nil, ErrUnavailable
	}
	var decoded struct {
		Condition struct {
			Code string `json:"code"`
			Text string `json:"text"`
		} `json:"condition"`
		Temperature struct {
			Value *float64 `json:"value"`
			Unit  string   `json:"unit"`
		} `json:"temperature"`
		Metadata struct {
			Attributions []string `json:"attributions"`
		} `json:"metadata"`
	}
	if err := json.Unmarshal(body, &decoded); err != nil {
		return nil, ErrUnavailable
	}
	temp := decoded.Temperature.Value
	code := decoded.Condition.Code
	text := strings.TrimSpace(decoded.Condition.Text)
	if temp == nil || math.IsNaN(*temp) || math.IsInf(*temp, 0) || *temp < -100 || *temp > 100 || decoded.Temperature.Unit != "°C" || len(code) != 3 || code[0] < '1' || code[0] > '9' || code[1] < '0' || code[1] > '9' || code[2] < '0' || code[2] > '9' || text == "" || len(text) > 256 || strings.IndexFunc(text, unicode.IsControl) >= 0 || len(decoded.Metadata.Attributions) == 0 || len(decoded.Metadata.Attributions) > 16 {
		return nil, ErrUnavailable
	}
	for _, attribution := range decoded.Metadata.Attributions {
		if strings.TrimSpace(attribution) == "" || len(attribution) > 2048 || strings.IndexFunc(attribution, unicode.IsControl) >= 0 {
			return nil, ErrUnavailable
		}
	}
	return &observation{current: Current{TemperatureC: *temp, ConditionCode: code, ConditionText: text, FetchedAt: p.now().UTC()}, attributions: decoded.Metadata.Attributions}, nil
}
