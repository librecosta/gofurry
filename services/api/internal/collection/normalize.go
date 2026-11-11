package collection

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"net/url"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

const projectionRules = "gofurry-evidence-1"

var digest = regexp.MustCompile(`^[0-9a-f]{64}$`)
var evidenceCode = regexp.MustCompile(`^[a-z0-9][a-z0-9_.:-]{0,95}$`)
var currencyCode = regexp.MustCompile(`^[A-Z]{3}$`)

type compact struct {
	Data    []byte
	Facts   map[string]jsontext.Value
	Ruleset string
	News    []newsItem
	Players *int64
}

func (c *compact) fact(key string, value any) {
	c.Facts[key], _ = json.Marshal(value, json.Deterministic(true))
}

func clean(s string, maxBytes int) string {
	s = strings.TrimSpace(strings.Map(func(r rune) rune {
		if unicode.IsControl(r) || unicode.Is(unicode.Cf, r) {
			return ' '
		}
		return r
	}, s))
	if len(s) > maxBytes {
		s = s[:maxBytes]
		for !utf8.ValidString(s) {
			s = s[:len(s)-1]
		}
	}
	return s
}

func publicURL(raw string) string {
	if len(raw) > 512 || strings.ContainsRune(raw, 92) || strings.ContainsFunc(raw, unicode.IsControl) {
		return ""
	}
	u, err := url.Parse(raw)
	if err != nil || u.Hostname() == "" || u.User != nil || u.Scheme != "https" && u.Scheme != "http" {
		return ""
	}
	u.RawQuery, u.Fragment, u.RawFragment, u.ForceQuery = "", "", "", false
	return u.String()
}

func scopeMatches(scope segmentScope, key string) bool {
	data, err := json.Marshal(scope, json.Deterministic(true))
	hash := sha256.Sum256(data)
	return err == nil && hex.EncodeToString(hash[:]) == key
}

type technology struct {
	ID         string   `json:"id"`
	Category   string   `json:"category"`
	Confidence string   `json:"confidence"`
	Evidence   []string `json:"evidence"`
}

type webMetadata struct {
	Ruleset      string       `json:"ruleset_version"`
	Technology   []technology `json:"technology"`
	EgressRegion string       `json:"egress_region,omitempty"`
}

type httpValue struct {
	Origin      string     `json:"origin"`
	FinalURL    string     `json:"final_url"`
	Status      int        `json:"status"`
	ContentType string     `json:"content_type,omitempty"`
	Redirects   []redirect `json:"redirects,omitempty"`
}

type redirect struct {
	Status   int    `json:"status"`
	To       string `json:"to,omitempty"`
	Followed bool   `json:"followed"`
}

type htmlValue struct {
	Title     string `json:"title,omitempty"`
	Language  string `json:"language,omitempty"`
	Canonical string `json:"canonical,omitempty"`
}

type dnsValue struct {
	Domain   string      `json:"domain"`
	RRType   string      `json:"type"`
	RCode    string      `json:"rcode"`
	Resolver string      `json:"resolver"`
	Vantage  string      `json:"vantage"`
	Records  []dnsRecord `json:"records"`
}

type dnsRecord struct {
	Owner      string `json:"owner"`
	Address    string `json:"address,omitempty"`
	Name       string `json:"name,omitempty"`
	Preference uint16 `json:"preference,omitempty"`
	SOA        *struct {
		Server string `json:"server"`
	} `json:"soa,omitempty"`
	ValueSHA256 string `json:"value_sha256,omitempty"`
	Tag         string `json:"tag,omitempty"`
	Flags       uint8  `json:"flags,omitempty"`
}

type tlsValue struct {
	Origin      string    `json:"origin"`
	Hostname    string    `json:"hostname"`
	Verified    bool      `json:"verified"`
	Issuer      string    `json:"issuer"`
	NotBefore   time.Time `json:"not_before"`
	NotAfter    time.Time `json:"not_after"`
	Certificate string    `json:"certificate_sha256"`
	SPKI        string    `json:"spki_sha256"`
}

type rdapValue struct {
	Domain          string   `json:"domain"`
	RegistryService string   `json:"registry_service"`
	Source          string   `json:"source"`
	Status          []string `json:"status,omitempty"`
	Nameservers     []string `json:"nameservers,omitempty"`
	Registrar       *struct {
		ID   string `json:"iana_id"`
		Name string `json:"name,omitempty"`
	} `json:"registrar,omitempty"`
	RegisteredAt *time.Time `json:"registered_at,omitempty"`
	ExpiresAt    *time.Time `json:"expires_at,omitempty"`
}

func boundedStrings(in []string, maxItems, maxBytes int) []string {
	values := make([]string, 0, min(maxItems, len(in)))
	for _, v := range in[:min(maxItems, len(in))] {
		if v = clean(v, maxBytes); v != "" {
			values = append(values, v)
		}
	}
	slices.Sort(values)
	return slices.Compact(values)
}

func normalizeWeb(o observation, key string) (compact, error) {
	c := compact{Facts: make(map[string]jsontext.Value), Ruleset: projectionRules}
	section := o.Scope.Dimensions["section"]
	if section != "dns" && o.State != "present" {
		return c, errProtocol
	}
	switch section {
	case "http":
		var v httpValue
		if json.Unmarshal(o.Data, &v) != nil || v.Origin != key || v.Status < 100 || v.Status > 599 || len(v.Redirects) > 8 {
			return c, errProtocol
		}
		v.FinalURL = publicURL(v.FinalURL)
		v.ContentType = clean(v.ContentType, 100)
		for i := range v.Redirects {
			v.Redirects[i].To = publicURL(v.Redirects[i].To)
		}
		c.fact("http.response", v)
	case "html":
		var v htmlValue
		if json.Unmarshal(o.Data, &v) != nil {
			return c, errProtocol
		}
		v.Title, v.Language, v.Canonical = clean(v.Title, 192), clean(v.Language, 32), publicURL(v.Canonical)
		c.fact("html.metadata", v)
	case "dns":
		var v dnsValue
		if json.Unmarshal(o.Data, &v) != nil || v.Domain != key || v.RRType != o.Scope.Dimensions["rrtype"] ||
			v.Resolver != o.Scope.Dimensions["resolver"] || v.Vantage != "recursive_doh" || v.Records == nil || len(v.Records) > 32 ||
			(o.State == "confirmed_empty" && (o.Evidence != "dns_response" || len(v.Records) != 0)) ||
			(o.State == "present" && (v.RCode != "noerror" || len(v.Records) == 0)) {
			return c, errProtocol
		}
		var records []jsontext.Value
		for _, record := range v.Records {
			if record.ValueSHA256 != "" && !digest.MatchString(record.ValueSHA256) {
				return c, errProtocol
			}
			record.Owner, record.Name, record.Address, record.Tag = clean(record.Owner, 253), clean(record.Name, 253), clean(record.Address, 48), clean(record.Tag, 16)
			if record.SOA != nil {
				record.SOA.Server = clean(record.SOA.Server, 253)
			}
			raw, _ := json.Marshal(record, json.Deterministic(true))
			records = append(records, raw)
		}
		slices.SortFunc(records, func(a, b jsontext.Value) int { return bytes.Compare(a, b) })
		records = slices.CompactFunc(records, func(a, b jsontext.Value) bool { return bytes.Equal(a, b) })
		full, _ := json.Marshal(records)
		checksum := sha256.Sum256(full)
		count, used := len(records), 0
		for i, record := range records {
			used += len(record) + 1
			if used > 3200 {
				records = records[:i]
				break
			}
		}
		normalized := struct {
			RCode    string           `json:"rcode"`
			Resolver string           `json:"resolver"`
			Vantage  string           `json:"vantage"`
			Records  []jsontext.Value `json:"records"`
			Count    int              `json:"record_count"`
			Hash     string           `json:"rrset_sha256"`
		}{v.RCode, v.Resolver, v.Vantage, records, count, hex.EncodeToString(checksum[:])}
		c.fact("dns.rrset", normalized)
	case "tls":
		var v tlsValue
		u, _ := url.Parse(key)
		if json.Unmarshal(o.Data, &v) != nil || u == nil || v.Origin != key || v.Hostname != u.Hostname() || !v.Verified ||
			!digest.MatchString(v.Certificate) || !digest.MatchString(v.SPKI) || !v.NotAfter.After(v.NotBefore) {
			return c, errProtocol
		}
		v.Issuer = clean(v.Issuer, 128)
		c.fact("tls.certificate", v)
	case "rdap":
		var v rdapValue
		if json.Unmarshal(o.Data, &v) != nil || v.Domain == "" || (key != v.Domain && !strings.HasSuffix(key, "."+v.Domain)) || v.Source != "iana_bootstrap" {
			return c, errProtocol
		}
		v.RegistryService = publicURL(v.RegistryService)
		v.Status, v.Nameservers = boundedStrings(v.Status, 12, 48), boundedStrings(v.Nameservers, 10, 160)
		if v.Registrar != nil {
			v.Registrar.ID, v.Registrar.Name = clean(v.Registrar.ID, 24), clean(v.Registrar.Name, 96)
		}
		c.fact("rdap.registration", v)
	default:
		return c, errProtocol
	}
	if section == "http" || section == "html" {
		var metadata webMetadata
		if json.Unmarshal(o.Data, &metadata) != nil || !evidenceCode.MatchString(metadata.Ruleset) || len(metadata.Technology) > 32 ||
			metadata.EgressRegion != "" && metadata.EgressRegion != o.Scope.EgressRegion {
			return c, errProtocol
		}
		c.Ruleset = metadata.Ruleset
		for _, t := range metadata.Technology {
			if !evidenceCode.MatchString(t.ID) || len(t.ID) > 64 || !evidenceCode.MatchString(t.Category) || !evidenceCode.MatchString(t.Confidence) ||
				len(t.Evidence) == 0 || len(t.Evidence) > 3 {
				return c, errProtocol
			}
			for _, code := range t.Evidence {
				if !evidenceCode.MatchString(code) {
					return c, errProtocol
				}
			}
			t.Evidence = boundedStrings(t.Evidence, 3, 96)
			c.fact("tech.detected."+t.ID, t)
		}
	}
	raw, err := json.Marshal(c.Facts, json.Deterministic(true))
	if err != nil || len(raw) > 6000 {
		return c, errProtocol
	}
	for _, value := range c.Facts {
		if len(value) > 5000 {
			return c, errProtocol
		}
	}
	c.Data = raw
	return c, nil
}

type steamProvenance struct {
	AppID uint32 `json:"app_id"`
	Tier  string `json:"tier"`
	URL   string `json:"source_url"`
}
type price struct {
	steamProvenance
	State   string `json:"state"`
	Amounts *struct {
		Currency string `json:"currency"`
		Initial  *int64 `json:"initial"`
		Final    *int64 `json:"final"`
		Discount *int   `json:"discount_percent"`
	} `json:"amounts,omitempty"`
}
type newsItem struct {
	GID         string    `json:"gid"`
	Title       string    `json:"title"`
	URL         string    `json:"url,omitempty"`
	Publisher   string    `json:"publisher,omitempty"`
	Feed        string    `json:"feed,omitempty"`
	PublishedAt time.Time `json:"published_at"`
}
type newsPage struct {
	steamProvenance
	Items []newsItem `json:"items"`
}
type players struct {
	steamProvenance
	Count  *int64 `json:"player_count"`
	Result int    `json:"result"`
}

func normalizeSteam(o observation, key string) (compact, error) {
	c := compact{Facts: map[string]jsontext.Value{}, Ruleset: projectionRules}
	var src steamProvenance
	if json.Unmarshal(o.Data, &src) != nil || strconv.FormatUint(uint64(src.AppID), 10) != key {
		return c, errProtocol
	}
	tier, address := "official_api", ""
	switch o.Capability {
	case "steam.details":
		tier, address = "observed_storefront", "https://store.steampowered.com/api/appdetails"
	case "steam.news":
		address = "https://api.steampowered.com/ISteamNews/GetNewsForApp/v2/"
	case "steam.players":
		address = "https://api.steampowered.com/ISteamUserStats/GetNumberOfCurrentPlayers/v1/"
	}
	if src.Tier != tier || src.URL != address {
		return c, errProtocol
	}
	if o.State != "present" && !(o.Scope.Dimensions["section"] == "news" && o.State == "confirmed_empty" && o.Evidence == "empty_page") {
		return c, errProtocol
	}
	var value any
	switch o.Scope.Dimensions["section"] {
	case "details":
		var v struct {
			steamProvenance
			Name string `json:"name"`
			Type string `json:"type,omitempty"`
		}
		if json.Unmarshal(o.Data, &v) != nil || v.Name == "" {
			return c, errProtocol
		}
		v.Name, v.Type = clean(v.Name, 192), clean(v.Type, 64)
		value = v
	case "media":
		var v struct {
			steamProvenance
			Header  string `json:"header,omitempty"`
			Capsule string `json:"capsule,omitempty"`
		}
		if json.Unmarshal(o.Data, &v) != nil {
			return c, errProtocol
		}
		v.Header, v.Capsule = publicURL(v.Header), publicURL(v.Capsule)
		value = v
	case "price":
		var v price
		if json.Unmarshal(o.Data, &v) != nil {
			return c, errProtocol
		}
		if v.State == "free" {
			if v.Amounts != nil {
				return c, errProtocol
			}
		} else {
			a := v.Amounts
			if a == nil || a.Initial == nil || a.Final == nil || *a.Initial < *a.Final || !currencyCode.MatchString(a.Currency) ||
				(a.Discount != nil && (*a.Discount < 0 || *a.Discount > 100)) ||
				!(v.State == "zero_price" && *a.Final == 0 || v.State == "priced" && *a.Final > 0) {
				return c, errProtocol
			}
		}
		value = v
	case "news":
		var v newsPage
		if json.Unmarshal(o.Data, &v) != nil || v.Items == nil || len(v.Items) > 10 || (len(v.Items) == 0) != (o.State == "confirmed_empty") {
			return c, errProtocol
		}
		seen := map[string]bool{}
		for _, item := range v.Items {
			id, err := strconv.ParseUint(item.GID, 10, 64)
			if err != nil || id == 0 || strconv.FormatUint(id, 10) != item.GID || item.PublishedAt.IsZero() {
				return c, errProtocol
			}
			if seen[item.GID] {
				continue
			}
			seen[item.GID] = true
			item.Title, item.Publisher, item.Feed, item.URL = clean(item.Title, 144), clean(item.Publisher, 64), clean(item.Feed, 48), publicURL(item.URL)
			if len(item.URL) > 220 {
				item.URL = ""
			}
			c.News = append(c.News, item)
		}
		v.Items = append([]newsItem{}, c.News...)
		value = v
	case "players":
		var v players
		if json.Unmarshal(o.Data, &v) != nil || v.Count == nil || *v.Count < 0 || v.Result != 1 || !o.Sample {
			return c, errProtocol
		}
		c.Players = v.Count
		value = v
	default:
		return c, errProtocol
	}
	raw, err := json.Marshal(value, json.Deterministic(true))
	if err != nil || len(raw) > 6000 {
		return c, errProtocol
	}
	c.Data = raw
	return c, nil
}
