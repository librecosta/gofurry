package legacyseed

import (
	"encoding/json/jsontext"
	"encoding/json/v2"
	"net"
	"net/netip"
	"net/url"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"golang.org/x/net/idna"
	_ "time/tzdata"
)

type values map[string]jsontext.Value

type source struct {
	Database string  `json:"database"`
	Table    string  `json:"table"`
	LegacyID int64   `json:"legacy_id"`
	Key      []int64 `json:"key"`
}

type record struct {
	Source    source         `json:"source"`
	Status    string         `json:"status"`
	Raw       values         `json:"raw"`
	Candidate map[string]any `json:"candidate"`
	Issues    []string       `json:"issues"`
}

func (v values) str(key string) string {
	var x string
	_ = json.Unmarshal(v[key], &x)
	return x
}

func (v values) number(key string) int64 {
	x, _ := strconv.ParseInt(string(v[key]), 10, 64)
	return x
}

func (v values) yes(key string) bool  { return string(v[key]) == "true" }
func (v values) null(key string) bool { return len(v[key]) == 0 || string(v[key]) == "null" }

func (r *record) issue(code string) {
	if !slices.Contains(r.Issues, code) {
		r.Issues = append(r.Issues, code)
	}
}

func (r *record) classify() {
	slices.Sort(r.Issues)
	switch {
	case slices.Contains(r.Issues, "invalid_legacy_id"):
		r.Status = "invalid"
	case slices.Contains(r.Issues, "duplicate_appid"), slices.Contains(r.Issues, "duplicate_code"), slices.Contains(r.Issues, "duplicate_address"):
		r.Status = "conflict"
	case r.Raw.yes("deleted"), !r.Raw.null("archived_at"), r.Raw.str("status") == "archived":
		r.Status = "archived"
	case len(r.Issues) > 0:
		r.Status = "needs_mapping"
	default:
		r.Status = "usable"
	}
}

var legacyZone = func() *time.Location {
	z, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		panic("missing embedded Asia/Shanghai timezone")
	}
	return z
}()

var businessCode = regexp.MustCompile("^[a-z0-9]+(-[a-z0-9]+)*$")
var asciiDomain = regexp.MustCompile("^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?$")

func cleanText(s string, limit int) (any, bool) {
	if !utf8.ValidString(s) {
		return nil, false
	}
	s = strings.Map(func(c rune) rune {
		switch {
		case unicode.IsSpace(c):
			return ' '
		case unicode.IsControl(c) || unicode.Is(unicode.Cf, c):
			return -1
		default:
			return c
		}
	}, s)
	s = strings.Join(strings.Fields(s), " ")
	if utf8.RuneCountInString(s) > limit || strings.ContainsAny(s, "<>") {
		return nil, false
	}
	if s == "" {
		return nil, true
	}
	return s, true
}

func cleanURL(raw string) string {
	s := strings.TrimSpace(raw)
	if len(s) > 2048 || !utf8.ValidString(s) || strings.ContainsAny(s, "\\<>") ||
		strings.IndexFunc(s, func(c rune) bool { return unicode.IsSpace(c) || unicode.IsControl(c) || unicode.Is(unicode.Cf, c) }) >= 0 {
		return ""
	}
	u, err := url.Parse(s)
	if err != nil || u.User != nil || u.Opaque != "" || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || u.Host == "" ||
		(u.Scheme != "http" && u.Scheme != "https") {
		return ""
	}
	host, port := strings.ToLower(strings.TrimSuffix(u.Hostname(), ".")), u.Port()
	if !utf8.ValidString(u.Path) || strings.ContainsAny(u.Path, "\\%<>") || strings.IndexFunc(u.Path, func(c rune) bool { return unicode.IsControl(c) || unicode.Is(unicode.Cf, c) }) >= 0 ||
		strings.Contains(strings.ToLower(u.EscapedPath()), "%2f") {
		return ""
	}
	for _, part := range strings.Split(u.Path, "/") {
		if part == ".." || part == "." {
			return ""
		}
	}
	if strings.HasSuffix(u.Host, ":") {
		return ""
	}
	if port != "" {
		n, err := strconv.Atoi(port)
		if err != nil || n < 1 || n > 65535 {
			return ""
		}
		port = strconv.Itoa(n)
	}
	if addr, err := netip.ParseAddr(host); err == nil {
		addr = addr.Unmap()
		if !addr.IsGlobalUnicast() || addr.IsPrivate() || addr.IsLinkLocalUnicast() || addr.Zone() != "" ||
			netip.MustParsePrefix("100.64.0.0/10").Contains(addr) {
			return ""
		}
		host = addr.String()
	} else {
		host, err = idna.Lookup.ToASCII(host)
		if err != nil || len(host) > 253 {
			return ""
		}
		labels := strings.Split(host, ".")
		if len(labels) < 2 || slices.Contains([]string{"local", "localhost", "internal", "test", "invalid", "onion", "arpa"}, labels[len(labels)-1]) ||
			strings.IndexFunc(labels[len(labels)-1], unicode.IsLetter) < 0 {
			return ""
		}
		for _, label := range labels {
			if !asciiDomain.MatchString(label) {
				return ""
			}
		}
	}
	u.Host = host
	if strings.Contains(host, ":") {
		u.Host = "[" + host + "]"
	}
	if port != "" && !(port == "443" && u.Scheme == "https") && !(port == "80" && u.Scheme == "http") {
		u.Host = net.JoinHostPort(host, port)
	}
	if u.Path == "/" {
		u.Path = ""
	}
	u.RawPath = ""
	return u.String()
}

func normalizedTime(raw, typ string) string {
	layout := time.RFC3339Nano
	zone := time.UTC
	if typ == "l" {
		layout, zone = "2006-01-02T15:04:05", legacyZone
	}
	t, err := time.ParseInLocation(layout, raw, zone)
	if typ == "z" && err != nil {
		t, err = time.Parse("2006-01-02T15:04:05Z07", raw)
	}
	if err != nil || (typ == "l" && t.Format("2006-01-02T15:04:05") != strings.Split(raw, ".")[0]) {
		return ""
	}
	return t.UTC().Format(time.RFC3339Nano)
}

type oldLink struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

func (r *record) jsonFields() {
	for _, field := range []string{"links", "resources", "groups", "developers", "publishers"} {
		r.Candidate[field] = nil
		if r.Raw.null(field) {
			continue
		}
		if field == "developers" || field == "publishers" {
			var list []jsontext.Value
			if json.Unmarshal(r.Raw[field], &list) != nil || len(list) > 128 {
				r.issue(field + "_invalid_json")
				continue
			}
			names := []string{}
			for _, item := range list {
				var name string
				err := json.Unmarshal(item, &name)
				clean, ok := cleanText(name, 255)
				if err != nil || !ok || clean == nil {
					r.issue(field + "_invalid_entry")
					continue
				}
				names = append(names, clean.(string))
			}
			r.Candidate[field] = names
			continue
		}
		var list []jsontext.Value
		if json.Unmarshal(r.Raw[field], &list) != nil || len(list) > 128 {
			r.issue(field + "_invalid_json")
			continue
		}
		links := []oldLink{}
		for _, item := range list {
			var link oldLink
			err := json.Unmarshal(item, &link, json.RejectUnknownMembers(true))
			label, ok := cleanText(link.Key, 255)
			dest := cleanURL(link.Value)
			if err != nil || !ok || label == nil || dest == "" {
				r.issue(field + "_invalid_entry")
				continue
			}
			links = append(links, oldLink{label.(string), dest})
		}
		r.Candidate[field] = links
	}
}

func normalize(s table, raw values) record {
	r := record{Source: source{Database: s.db, Table: s.name}, Raw: raw, Candidate: map[string]any{}, Issues: []string{}}
	for _, f := range strings.Split(s.keys, ",") {
		x := raw.number(f)
		r.Source.Key = append(r.Source.Key, x)
		if x <= 0 {
			r.issue("invalid_legacy_id")
		}
	}
	r.Source.LegacyID = r.Source.Key[0]
	for _, field := range s.fields() {
		f, typ, _ := strings.Cut(field, ":")
		if string(raw[f]) == `{"source_value_oversize": true}` || string(raw[f]) == `{"source_value_oversize":true}` {
			r.issue(f + "_oversize")
		}
		if raw.null(f) {
			continue
		}
		if typ == "l" || typ == "z" {
			out := normalizedTime(raw.str(f), typ)
			if out == "" {
				r.issue(f + "_invalid_time")
				continue
			}
			r.Candidate[f+"_utc"] = out
		}
	}
	if raw["name_en"] != nil {
		for _, f := range []string{"name", "name_en", "info", "info_en"} {
			limit := 8000
			if strings.HasPrefix(f, "name") {
				limit = 255
			}
			x, ok := cleanText(raw.str(f), limit)
			r.Candidate[f] = x
			switch {
			case !ok:
				r.issue(f + "_invalid_text")
			case x == nil:
				r.issue(f + "_missing")
			}
		}
		r.Candidate["translation_languages"] = map[string]string{"name": "zh-CN", "info": "zh-CN", "name_en": "en", "info_en": "en"}
	}
	for _, f := range []string{"weight", "priority", "sort_order", "export_order", "role", "showcase_eligible", "status"} {
		if !raw.null(f) {
			r.Candidate[f] = raw[f]
		}
	}
	if raw["code"] != nil {
		r.Candidate["legacy_code"] = nil
		code := raw.str("code")
		if len(code) > 64 || !businessCode.MatchString(code) {
			r.issue("invalid_code")
		} else {
			r.Candidate["legacy_code"] = code
		}
		if raw.number("code_count") > 1 {
			r.issue("duplicate_code")
		}
	}
	switch s.name {
	case "gfn_site":
		r.Candidate["first_published_at"] = nil
		r.Candidate["legacy_deleted"] = raw.yes("deleted")
		r.Candidate["legacy_public_sql_eligible"] = !raw.yes("deleted")
		switch raw.str("nsfw") {
		case "1":
			r.Candidate["adult_evidence"] = "legacy_adult"
		case "0":
			r.Candidate["adult_evidence"] = "legacy_non_adult_flag"
		default:
			r.Candidate["adult_evidence"] = "unknown"
			r.issue("adult_flag_unknown")
		}
		if raw.yes("deleted") == raw.null("deleted_at") {
			r.issue("site_deletion_metadata_incomplete")
		}
	case "gfn_site_group":
		r.Candidate["new_classification"] = "unmapped"
		r.issue("classification_unmapped")
		if (strings.TrimSpace(raw.str("name")) != "" && raw.number("zh_name_count") > 1) ||
			(strings.TrimSpace(raw.str("name_en")) != "" && raw.number("en_name_count") > 1) {
			r.issue("classification_duplicate_label")
		}
	case "gfn_site_group_map":
		if raw.number("pair_count") > 1 {
			r.issue("duplicate_site_group_membership")
		}
	case "gfn_featured_site":
		if raw.number("featured_count") > 1 {
			r.issue("repeated_site_featured_record")
		}
	case "gfg_game":
		r.Candidate["first_published_at"] = nil
		r.Candidate["adult_evidence"] = "unknown"
		r.Candidate["legacy_public_filter"] = "not_flagged_adult"
		if raw.yes("legacy_adult") {
			r.Candidate["adult_evidence"] = "legacy_adult"
			r.Candidate["legacy_public_filter"] = "excluded_by_adult_tag"
		}
		r.Candidate["steam_appid"] = nil
		switch id := raw.number("appid"); {
		case id == 0:
			r.issue("steam_appid_missing")
		case id < 0 || id > 4294967295:
			r.issue("steam_appid_invalid")
		case raw.number("appid_count") > 1:
			r.issue("duplicate_appid")
		default:
			r.Candidate["steam_appid"] = id
		}
		r.Candidate["manual_release_text"], _ = cleanText(raw.str("release_date"), 1024)
		if !raw.null("release_date") && raw.str("release_date") != "" && r.Candidate["manual_release_text"] == nil {
			r.issue("release_date_invalid_text")
		}
		r.jsonFields()
	case "gfg_game_tag":
		role := raw.str("role")
		if !slices.Contains([]string{"normal", "primary", "secondary"}, role) {
			r.issue("invalid_tag_role")
		}
		if (role == "primary" || role == "secondary") && raw.number("role_count") > 1 {
			r.issue("duplicate_tag_role")
		}
	case "gfg_game_collection":
		if !slices.Contains([]string{"draft", "published", "archived"}, raw.str("status")) {
			r.issue("invalid_collection_status")
		}
		if (raw.str("status") == "published" && raw.null("published_at")) ||
			(raw.str("status") == "archived" && raw.null("archived_at")) ||
			(raw.str("status") != "archived" && !raw.null("archived_at")) {
			r.issue("collection_lifecycle_conflict")
		}
	case "gfg_game_collection_item":
		if raw.number("member_count") > 512 {
			r.issue("collection_manual_limit_exceeded")
		}
		if raw.yes("explicitly_excluded") {
			r.issue("collection_manual_exclusion_conflict")
		}
	case "gfg_game_collection_home_slot":
		if n := raw.number("slot"); n < 1 || n > 5 {
			r.issue("invalid_home_slot")
		}
		if raw.number("slot_count") > 1 {
			r.issue("multiple_collection_slots")
		}
	case "gfg_game_first_available":
		r.Candidate["manual_release_evidence"] = true
		if !slices.Contains([]string{"day", "month", "quarter", "year"}, raw.str("precision")) ||
			raw.str("window_start") > raw.str("window_end") || raw.number("release_year") < 1 ||
			(raw.str("precision") == "day" && (raw.str("exact_date") == "" || raw.str("exact_date") != raw.str("window_start") || raw.str("exact_date") != raw.str("window_end"))) {
			r.issue("manual_release_date_conflict")
		}
	}
	return r
}
