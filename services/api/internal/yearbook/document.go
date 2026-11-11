package yearbook

import (
	"cmp"
	"crypto/sha256"
	"encoding/hex"
	json "encoding/json/v2"
	"math/bits"
	"net"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"time"
	"unicode"
	"uuid"

	"github.com/librecosta/gofurry/services/api/internal/database/dbgen"
)

const (
	algorithm = 1
	minDays   = 14
	minSpan   = 28
)

type Name struct {
	Locale string `json:"locale"`
	Name   string `json:"name"`
}

type DateRange struct {
	From string `json:"from"`
	To   string `json:"to"`
}

type CoverageSource struct {
	Host       string `json:"host"`
	Capability string `json:"capability"`
	Scopes     int    `json:"scopes"`
}

type Coverage struct {
	PeriodStart    string           `json:"period_start"`
	PeriodEnd      string           `json:"period_end"`
	RecordedFrom   string           `json:"recorded_from,omitempty"`
	RecordedTo     string           `json:"recorded_to,omitempty"`
	Days           int              `json:"days"`
	PeriodDays     int              `json:"period_days"`
	PartialYear    bool             `json:"partial_year"`
	Months         [12]uint32       `json:"months"`
	Unknown        []DateRange      `json:"unknown"`
	Sources        []CoverageSource `json:"sources"`
	SourcesOmitted int              `json:"sources_omitted"`
}

type Fact struct {
	Kind       string   `json:"kind"`
	Value      string   `json:"value"`
	Previous   string   `json:"previous,omitempty"`
	Host       string   `json:"host"`
	Capability string   `json:"capability"`
	Vantage    string   `json:"vantage"`
	Dimension  string   `json:"dimension,omitempty"`
	Ruleset    string   `json:"ruleset"`
	Evidence   string   `json:"evidence"`
	Signals    []string `json:"signals,omitempty"`
	Confidence string   `json:"confidence"`
	Detected   string   `json:"detected"`
	Observed   string   `json:"observed"`
	Reference  string   `json:"reference"`
	PriorKnown bool     `json:"prior_known,omitempty"`
}

type Document struct {
	Algorithm       int      `json:"algorithm"`
	Names           []Name   `json:"names"`
	Coverage        Coverage `json:"coverage"`
	Timeline        []Fact   `json:"timeline"`
	YearEnd         []Fact   `json:"year_end"`
	TimelineOmitted int      `json:"timeline_omitted"`
	YearEndOmitted  int      `json:"year_end_omitted"`
}

type scopeIdentity struct {
	binding    uuid.UUID
	capability string
	scope      string
}

type scopedFact struct {
	scopeIdentity
	key string
}

var (
	code = regexp.MustCompile(`^[a-z0-9][a-z0-9_.:-]{0,95}$`)
	hash = regexp.MustCompile(`^[0-9a-f]{64}$`)
	host = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?(?:\.[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?)+$`)
)

func sourceHost(raw string) string {
	if strings.Contains(raw, "://") {
		u, err := url.Parse(raw)
		if err != nil || u.User != nil || (u.Scheme != "https" && u.Scheme != "http") {
			return ""
		}
		raw = u.Hostname()
	}
	raw = strings.ToLower(strings.TrimSuffix(raw, "."))
	if len(raw) > 253 || !host.MatchString(raw) || net.ParseIP(raw) != nil ||
		strings.HasSuffix(raw, ".local") || strings.HasSuffix(raw, ".internal") || strings.HasSuffix(raw, ".localhost") {
		return ""
	}
	return raw
}

func editorialNames(rows []dbgen.YearbookSiteNamesRow) []Name {
	names := make([]Name, 0, len(rows))
	for _, row := range rows {
		name := strings.TrimSpace(strings.Map(func(r rune) rune {
			if unicode.IsControl(r) || unicode.Is(unicode.Cf, r) {
				return -1
			}
			return r
		}, row.Name))
		if (row.Locale == "zh" || row.Locale == "en") && name != "" && len([]rune(name)) <= 160 {
			names = append(names, Name{row.Locale, name})
		}
	}
	return names
}

func summarizeCoverage(start, end time.Time, rows []dbgen.YearbookCoverageRow) (Coverage, map[scopeIdentity]bool) {
	c := Coverage{PeriodStart: day(start), PeriodEnd: day(end), PeriodDays: int(end.Sub(start).Hours() / 24),
		Unknown: []DateRange{}, Sources: []CoverageSource{}}
	type bySource struct{ host, capability string }
	sourceScopes := map[bySource]map[string]bool{}
	verified := make(map[scopeIdentity]bool, len(rows))
	for _, row := range rows {
		name := sourceHost(row.SourceKey)
		m := row.Month.Time
		if name == "" || !row.Month.Valid || m.Year() != start.Year() || row.DayMask <= 0 {
			continue
		}
		days := m.AddDate(0, 1, -1).Day()
		if row.DayMask >= int64(1)<<days {
			continue
		}
		c.Months[m.Month()-1] |= uint32(row.DayMask)
		verified[scopeIdentity{row.BindingID, row.Capability, row.ScopeKey}] = true
		key := bySource{name, row.Capability}
		if sourceScopes[key] == nil {
			sourceScopes[key] = map[string]bool{}
		}
		sourceScopes[key][row.ScopeKey] = true
	}
	for _, mask := range c.Months {
		c.Days += bits.OnesCount32(mask)
	}
	for date := start; date.Before(end); date = date.AddDate(0, 0, 1) {
		if c.Months[date.Month()-1]&(uint32(1)<<(date.Day()-1)) != 0 {
			if c.RecordedFrom == "" {
				c.RecordedFrom = day(date)
			}
			c.RecordedTo = day(date)
			continue
		}
		if n := len(c.Unknown); n > 0 && c.Unknown[n-1].To == day(date.AddDate(0, 0, -1)) {
			c.Unknown[n-1].To = day(date)
		} else {
			c.Unknown = append(c.Unknown, DateRange{day(date), day(date)})
		}
	}
	c.PartialYear = c.RecordedFrom != day(start) || c.RecordedTo != day(end.AddDate(0, 0, -1))
	for key, scopes := range sourceScopes {
		c.Sources = append(c.Sources, CoverageSource{key.host, key.capability, len(scopes)})
	}
	slices.SortFunc(c.Sources, func(a, b CoverageSource) int {
		return cmp.Or(cmp.Compare(a.Host, b.Host), cmp.Compare(a.Capability, b.Capability))
	})
	if len(c.Sources) > 24 {
		c.SourcesOmitted = len(c.Sources) - 24
		c.Sources = c.Sources[:24]
	}
	return c, verified
}

type publicValue struct {
	text, material, confidence string
	signals                    []string
}

func technologyValue(key string, raw []byte) publicValue {
	var v struct {
		ID         string   `json:"id"`
		Confidence string   `json:"confidence"`
		Evidence   []string `json:"evidence"`
	}
	if json.Unmarshal(raw, &v) != nil || !code.MatchString(v.ID) || key != "tech.detected."+v.ID ||
		(v.Confidence != "high" && v.Confidence != "medium" && v.Confidence != "low") ||
		len(v.Evidence) < 1 || len(v.Evidence) > 3 {
		return publicValue{}
	}
	for _, signal := range v.Evidence {
		if !code.MatchString(signal) {
			return publicValue{}
		}
	}
	slices.Sort(v.Evidence)
	return publicValue{v.ID, v.ID, v.Confidence, slices.Compact(v.Evidence)}
}

func factValue(kind, key string, raw []byte) publicValue {
	switch kind {
	case "technology":
		return technologyValue(key, raw)
	case "http":
		var v struct {
			Origin string `json:"origin"`
			Final  string `json:"final_url"`
		}
		if json.Unmarshal(raw, &v) == nil {
			if final := sourceHost(v.Final); final != "" {
				return publicValue{final, final, "verified", nil}
			}
			if origin := sourceHost(v.Origin); origin != "" {
				return publicValue{origin, "", "verified", nil}
			}
		}
	case "dns":
		var v struct {
			Checksum string `json:"rrset_sha256"`
			Vantage  string `json:"vantage"`
		}
		if json.Unmarshal(raw, &v) == nil && v.Vantage == "recursive_doh" && hash.MatchString(v.Checksum) {
			return publicValue{v.Checksum[:16], v.Checksum, "verified", nil}
		}
	case "tls":
		var v struct {
			Verified bool   `json:"verified"`
			Cert     string `json:"certificate_sha256"`
			SPKI     string `json:"spki_sha256"`
		}
		if json.Unmarshal(raw, &v) == nil && v.Verified && hash.MatchString(v.Cert) && hash.MatchString(v.SPKI) {
			return publicValue{v.Cert[:16], v.Cert + v.SPKI, "verified", nil}
		}
	case "rdap":
		var v struct {
			Domain string `json:"domain"`
			Source string `json:"source"`
		}
		if json.Unmarshal(raw, &v) == nil && v.Source == "iana_bootstrap" {
			if domain := sourceHost(v.Domain); domain != "" {
				return publicValue{domain, domain, "verified", nil}
			}
		}
	}
	return publicValue{}
}

func classifyFact(row dbgen.YearbookFactsRow) (string, string, string) {
	var scope struct {
		Egress     string            `json:"egress_region"`
		Dimensions map[string]string `json:"dimensions"`
	}
	if json.Unmarshal(row.Scope, &scope) != nil || !code.MatchString(scope.Egress) || !hash.MatchString(row.ScopeKey) {
		return "", "", ""
	}
	section := scope.Dimensions["section"]
	switch {
	case row.FactKey == "http.response" && row.Capability == "web.http" && section == "http":
		return "http", scope.Egress, ""
	case strings.HasPrefix(row.FactKey, "tech.detected.") && row.Capability == "web.http" && (section == "html" || section == "http"):
		return "technology", scope.Egress, section
	case row.FactKey == "dns.rrset" && row.Capability == "web.dns" && section == "dns":
		rr := scope.Dimensions["rrtype"]
		if slices.Contains([]string{"a", "aaaa", "caa", "cname", "mx", "ns", "soa", "txt"}, rr) {
			return "dns", scope.Egress, strings.ToUpper(rr)
		}
	case row.FactKey == "tls.certificate" && (row.Capability == "web.tls" || row.Capability == "web.http") && section == "tls":
		return "tls", scope.Egress, ""
	case row.FactKey == "rdap.registration" && row.Capability == "web.rdap" && section == "rdap":
		return "rdap", scope.Egress, ""
	}
	return "", "", ""
}

func publicFact(row dbgen.YearbookFactsRow, start time.Time) (Fact, publicValue, bool) {
	kind, vantage, dimension := classifyFact(row)
	name := sourceHost(row.ExternalKey)
	if kind == "" || name == "" || !code.MatchString(row.Ruleset) || !row.SourceObservedAt.Valid || !row.FirstObservedAt.Valid {
		return Fact{}, publicValue{}, false
	}
	if row.EventKind == "withdrawn" && row.SourceObservationID == nil && row.Evidence == "scope_withdrawn" {
		return Fact{}, publicValue{}, true
	}
	value := factValue(kind, row.FactKey, row.Value)
	if value.text == "" || row.SourceObservationID == nil || !hash.MatchString(row.ContentHash) {
		return Fact{}, publicValue{}, false
	}
	evidence := row.Evidence
	if evidence == "" {
		evidence = "verified_observation"
	}
	if !code.MatchString(evidence) {
		return Fact{}, publicValue{}, false
	}
	ref := sha256.Sum256([]byte(row.ID.String() + row.ScopeKey + row.ContentHash))
	fact := Fact{Kind: kind, Value: value.text, Host: name, Capability: row.Capability, Vantage: vantage, Dimension: dimension,
		Ruleset: row.Ruleset, Evidence: evidence, Signals: value.signals, Confidence: value.confidence,
		Detected: instant(row.FirstObservedAt.Time), Observed: instant(row.SourceObservedAt.Time),
		Reference: hex.EncodeToString(ref[:10]), PriorKnown: row.SourceObservedAt.Time.Before(start)}
	return fact, value, true
}

func factsForYear(doc *Document, rows []dbgen.YearbookFactsRow, confirmed map[scopeIdentity]bool, start, end time.Time) bool {
	latest := map[scopedFact]Fact{}
	priorBaselines := map[scopedFact]bool{}
	timeline := []Fact{}
	hasBaseline := false
	var primary dbgen.YearbookFactsRow
	var primaryHost string
	for _, row := range rows {
		if !row.SourceObservedAt.Time.Before(end) || !row.ReceivedAt.Valid {
			continue
		}
		fact, value, valid := publicFact(row, start)
		if !valid {
			continue
		}
		scope := scopeIdentity{row.BindingID, row.Capability, row.ScopeKey}
		key := scopedFact{scope, row.FactKey}
		within := !row.FirstObservedAt.Time.Before(start)
		if row.EventKind == "withdrawn" {
			delete(latest, key)
			delete(priorBaselines, key)
			continue
		}
		atCutoff := !row.EffectiveUntil.Valid || !row.EffectiveUntil.Time.Before(end)
		if atCutoff {
			latest[key] = fact
		}
		if row.EventKind == "baseline" && confirmed[scope] {
			if within {
				hasBaseline = true
			} else if atCutoff {
				priorBaselines[key] = true
			}
		}
		if fact.Kind == "http" && row.Purpose == "primary" {
			if within && primaryHost != "" && primaryHost != fact.Host && primary.EffectiveUntil.Valid &&
				!primary.EffectiveUntil.Time.After(row.EffectiveFrom.Time) {
				change := fact
				change.Kind, change.Value, change.Previous = "domain", fact.Host, primaryHost
				timeline = append(timeline, change)
			}
			if primaryHost == "" || !row.EffectiveFrom.Time.Before(primary.EffectiveFrom.Time) {
				primary, primaryHost = row, fact.Host
			}
		}
		if !within || row.EventKind != "change" || fact.Kind == "rdap" {
			continue
		}
		before := factValue(fact.Kind, row.FactKey, row.PriorValue)
		if fact.Kind == "technology" {
			if row.PriorRuleset != row.Ruleset || before.text != "" || value.confidence != "high" {
				continue
			}
		} else if before.material == "" || value.material == "" || before.material == value.material {
			continue
		}
		fact.Previous = before.text
		timeline = append(timeline, fact)
	}
	doc.Timeline, doc.TimelineOmitted = capped(timeline, 40)
	endFacts := make([]Fact, 0, len(latest))
	for _, fact := range latest {
		endFacts = append(endFacts, fact)
	}
	slices.SortFunc(endFacts, func(a, b Fact) int {
		return cmp.Or(cmp.Compare(b.Observed, a.Observed), cmp.Compare(a.Host, b.Host), cmp.Compare(a.Reference, b.Reference))
	})
	doc.YearEnd, doc.YearEndOmitted = capped(endFacts, 24)
	return hasBaseline || len(priorBaselines) > 0
}

func capped(facts []Fact, limit int) ([]Fact, int) {
	return facts[:min(len(facts), limit)], max(0, len(facts)-limit)
}

func day(t time.Time) string     { return t.UTC().Format(time.DateOnly) }
func instant(t time.Time) string { return t.UTC().Format(time.RFC3339) }
