package collection

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
	"uuid"

	"github.com/librecosta/gofurry/services/api/internal/platform"
)

var (
	errProtocol       = errors.New("Harvest response failed protocol validation")
	errNetwork        = errors.New("Harvest connection or response unavailable")
	errBusy           = errors.New("collection operation already active")
	errSchema         = errors.New("Harvest output schema requires consumer upgrade")
	errAmbiguous      = errors.New("Harvest schedule outcome unknown; reconciliation found no matching schedule")
	errRemoteConflict = errors.New("Harvest resource differs from the persisted collection intent")
)

type remoteStatus int

func (s remoteStatus) Error() string { return fmt.Sprintf("Harvest HTTP %d", int(s)) }

type client struct {
	base, writer, reader string
	http                 *http.Client
}

func newClient(c platform.HarvestConfig) *client {
	tr := &http.Transport{
		DialContext:     (&net.Dialer{Timeout: 3 * time.Second, KeepAlive: 30 * time.Second}).DialContext,
		TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12}, ForceAttemptHTTP2: true,
		TLSHandshakeTimeout: 3 * time.Second, ResponseHeaderTimeout: 8 * time.Second,
		MaxIdleConns: 2, MaxIdleConnsPerHost: 2, MaxConnsPerHost: 2, IdleConnTimeout: 30 * time.Second,
	}
	return &client{strings.TrimSuffix(c.URL, "/"), c.WriterToken, c.ReaderToken,
		&http.Client{Transport: tr, Timeout: 12 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}
}

func (c *client) call(ctx context.Context, token, method, path string, query url.Values, input, output any, expected int) error {
	if token == "" {
		return errors.New("Harvest role credential unavailable")
	}
	target := c.base + "/api/v1/" + path
	if len(query) != 0 {
		target += "?" + query.Encode()
	}
	var body []byte
	if input != nil {
		var err error
		body, err = json.Marshal(input)
		if err != nil {
			return errProtocol
		}
	}
	req, err := http.NewRequestWithContext(ctx, method, target, bytes.NewReader(body))
	if err != nil {
		return errProtocol
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")
	if input != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	res, err := c.http.Do(req)
	if err != nil {
		return errNetwork
	}
	defer res.Body.Close()
	if res.StatusCode != expected {
		// Never propagate a remote body, URL or authorization value into logs.
		_, _ = io.Copy(io.Discard, io.LimitReader(res.Body, 4096))
		return remoteStatus(res.StatusCode)
	}
	const max = 2 << 20
	data, err := io.ReadAll(io.LimitReader(res.Body, max+1))
	if err != nil {
		return errNetwork
	}
	if len(data) > max || len(data) == 0 || data[0] != '{' || json.Unmarshal(data, output) != nil {
		return errProtocol
	}
	return nil
}

type targetKey struct {
	Namespace    string `json:"namespace"`
	Provider     string `json:"provider"`
	ResourceType string `json:"resource_type"`
	ExternalKey  string `json:"external_key"`
}
type targetView struct {
	targetKey
	ID    uuid.UUID `json:"id"`
	State string    `json:"state"`
}

type scope struct {
	Locale       string `json:"locale,omitempty"`
	Region       string `json:"region,omitempty"`
	EgressRegion string `json:"egress_region,omitempty"`
}
type segmentScope struct {
	Namespace string `json:"namespace"`
	scope
	Dimensions map[string]string `json:"dimensions,omitempty"`
}
type request struct {
	TargetID   uuid.UUID      `json:"target_id"`
	Capability string         `json:"capability"`
	Schema     int32          `json:"request_schema"`
	Params     jsontext.Value `json:"params"`
	Scope      scope          `json:"scope"`
}
type recurrence struct {
	Interval int32     `json:"interval_seconds"`
	Anchor   time.Time `json:"anchor_at"`
	Policy   string    `json:"policy"`
	Cron     string    `json:"cron,omitempty"`
}
type scheduleSpec struct {
	Request    request    `json:"request"`
	Recurrence recurrence `json:"recurrence"`
	Enabled    bool       `json:"enabled"`
}
type scheduleView struct {
	scheduleSpec
	ID        uuid.UUID `json:"id"`
	Namespace string    `json:"namespace"`
	Revision  int64     `json:"revision"`
}
type schedulePage struct {
	Items []scheduleView `json:"items"`
	After *uuid.UUID     `json:"after"`
}
type identity struct {
	TargetID   uuid.UUID `json:"target_id"`
	Capability string    `json:"capability"`
	Schema     int32     `json:"output_schema"`
	ScopeKey   string    `json:"scope_key"`
}
type observation struct {
	identity
	ID         uuid.UUID      `json:"id"`
	Scope      segmentScope   `json:"scope"`
	State      string         `json:"state"`
	ObservedAt time.Time      `json:"observed_at"`
	ReceivedAt time.Time      `json:"received_at"`
	Data       jsontext.Value `json:"data"`
	Hash       string         `json:"content_hash"`
	Source     string         `json:"source"`
	Evidence   string         `json:"evidence"`
	Sample     bool           `json:"sample"`
}
type currentItem struct {
	observation
	Position  int64     `json:"position"`
	Confirmed time.Time `json:"confirmed_at"`
}
type currentPage struct {
	Items []currentItem `json:"items"`
	After string        `json:"after"`
	Epoch uuid.UUID     `json:"epoch"`
	Head  string        `json:"head_cursor"`
}
type event struct {
	identity
	Position    int64        `json:"position"`
	Action      string       `json:"action"`
	Committed   time.Time    `json:"committed_at"`
	Observation *observation `json:"observation"`
	Tombstone   segmentScope `json:"tombstone_scope"`
}
type changesPage struct {
	Items []event   `json:"items"`
	Epoch uuid.UUID `json:"epoch"`
	Next  string    `json:"next_cursor"`
	Head  string    `json:"head_cursor"`
	More  bool      `json:"has_more"`
}
type resetView struct {
	ID       uuid.UUID `json:"reset"`
	Epoch    uuid.UUID `json:"epoch"`
	Baseline string    `json:"baseline_cursor"`
	Deadline time.Time `json:"deadline"`
}
type ackInput struct {
	Stream string     `json:"stream"`
	Cursor string     `json:"cursor"`
	Reset  *uuid.UUID `json:"reset,omitempty"`
}
type ackView struct {
	ID            uuid.UUID `json:"id"`
	Stream        string    `json:"stream"`
	Epoch         uuid.UUID `json:"epoch"`
	ACK           string    `json:"ack"`
	ResetRequired bool      `json:"reset_required"`
}

func v7(id uuid.UUID) bool  { return id[6]>>4 == 7 && id[8]>>6 == 2 }
func tokenOK(s string) bool { return s != "" && len(s) <= 1024 }
