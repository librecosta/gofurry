package legacyimport

import (
	"cmp"
	"context"
	"encoding/json/jsontext"
	"encoding/json/v2"
	"errors"
	"net/url"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/librecosta/gofurry/services/api/internal/catalog"
	"github.com/librecosta/gofurry/services/api/internal/collection"
	"github.com/librecosta/gofurry/services/api/internal/legacyseed"
	"github.com/librecosta/gofurry/services/api/internal/storage"
)

var codePattern = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

type seedRow struct {
	key
	raw       map[string]jsontext.Value
	candidate map[string]jsontext.Value
}

func field[T any](fields map[string]jsontext.Value, name string) T {
	var x T
	_ = json.Unmarshal(fields[name], &x)
	return x
}
func validRole(role string) bool { return role == "normal" || role == "primary" || role == "secondary" }

type sourceResult struct {
	Source       key      `json:"source"`
	Disposition  string   `json:"disposition"`
	Issues       []string `json:"issues,omitempty"`
	DecisionRefs []string `json:"decision_refs,omitempty"`
}
type relation struct {
	Key       key
	Reference key
	Role      string
	Position  int
}
type intent struct {
	Key            key
	Editorial      catalog.Editorial
	Taxonomy       taxonomyValue
	Addresses      []catalog.Address
	AddressRows    []key
	GroupRows      []key
	Sources        []catalog.Source
	Code           string
	Sort           int32
	Category       key
	ArchivedAt     time.Time
	Adult          bool
	Archive        bool
	Publish        bool
	PublishProof   proof
	Link           *linkValue
	Alias          bool
	LinkProof      proof
	DecisionRefs   []string
	Logo           string
	Relations      []relation
	RelationIssue  string
	IdentityIssue  string
	LogoIssue      string
	LifecycleIssue string
	SiteHintURL    string
}

type blueprint struct {
	Items []intent
	Rows  map[string]sourceResult
}

func mapLoc(b *blueprint, row seedRow) []catalog.Localization {
	out := []catalog.Localization{}
	for _, pair := range [][2]string{{"zh", ""}, {"en", "_en"}} {
		name := field[string](row.candidate, "name"+pair[1])
		if name == "" {
			continue
		}
		l := catalog.Localization{Locale: pair[0], Name: name, Summary: field[string](row.candidate, "info"+pair[1])}
		check := func() bool {
			_, err := catalog.NormalizeEditorial(catalog.Editorial{Slug: "validation", Localizations: []catalog.Localization{l}})
			return err == nil
		}
		if !check() {
			l.Summary = ""
			if !check() {
				b.issue(row.key, "target_invalid_name_"+pair[0])
				continue
			}
			b.issue(row.key, "target_invalid_summary_"+pair[0])
		}
		out = append(out, l)
	}
	return out
}

func (b *blueprint) issue(k key, code string) {
	id := k.String()
	row := b.Rows[id]
	if !slices.Contains(row.Issues, code) {
		row.Issues = append(row.Issues, code)
	}
	b.Rows[id] = row
}

func fromSource(s legacyseed.Source) key { return makeKey(s.Table, s.Key...) }

func build(ctx context.Context, bundle *legacyseed.Bundle, ds []decision) (blueprint, error) {
	b := blueprint{Rows: map[string]sourceResult{}}
	rows := map[string]seedRow{}
	order := []key{}
	err := bundle.Content(ctx, func(record legacyseed.Record) error {
		k := fromSource(record.Source)
		id := k.String()
		row := seedRow{key: k, raw: record.Raw}
		if json.Unmarshal(record.Candidate, &row.candidate) != nil || rows[id].key.Table != "" {
			return errors.New("invalid or duplicate seed source key")
		}
		rows[id] = row
		order = append(order, k)
		b.Rows[id] = sourceResult{Source: k, Disposition: "evidence_only", Issues: record.Issues}
		if !k.valid() {
			b.set(k, "deferred")
		}
		return nil
	})
	if err != nil {
		return b, err
	}
	if err = bundle.Rows(ctx, "reports/excluded-release-rows.ndjson", func(raw jsontext.Value) error {
		var row struct {
			Source legacyseed.Source `json:"source"`
			Code   string            `json:"code"`
		}
		if decode(raw, &row) != nil {
			return errors.New("invalid seed exclusion")
		}
		k := fromSource(row.Source)
		if rows[k.String()].key.Table != "" {
			return errors.New("duplicate upstream exclusion")
		}
		b.Rows[k.String()] = sourceResult{Source: k, Disposition: "excluded_upstream", Issues: []string{row.Code}}
		return nil
	}); err != nil {
		return b, err
	}
	review, err := reviewDecisions(ds, rows)
	if err != nil {
		return b, err
	}
	for id, decision := range review {
		result := b.Rows[id]
		result.DecisionRefs = decision.refs
		b.Rows[id] = result
	}

	sources := map[int64]legacyseed.MappingCandidate{}
	err = bundle.Rows(ctx, "reports/mapping-candidates.ndjson", func(raw jsontext.Value) error {
		var c legacyseed.MappingCandidate
		if decode(raw, &c) != nil {
			return errors.New("invalid seed source candidate")
		}
		if c.Source.Table == games {
			sources[c.Source.LegacyID] = c
		}
		return nil
	})
	if err != nil {
		return b, err
	}

	logos := map[int64]legacyseed.Logo{}
	validated := map[string]storage.SiteAsset{}
	err = bundle.Rows(ctx, "assets/site-logo-index.ndjson", func(raw jsontext.Value) error {
		var l legacyseed.Logo
		if decode(raw, &l) != nil {
			return errors.New("invalid seed logo index")
		}
		logos[l.SiteID] = l
		if l.Status != "ready" {
			return nil
		}
		if _, ok := validated[l.SHA]; !ok {
			file, err := bundle.LogoBytes(l.SHA)
			if err != nil {
				return err
			}
			asset, err := storage.ValidateSiteAsset(storage.SiteLogo, file)
			if err != nil {
				return errors.New("invalid verified Site Logo bytes")
			}
			validated[l.SHA] = asset
		}
		a := validated[l.SHA]
		if a.SHA256 != l.SHA || a.Key != l.Key || a.Width != l.Width || a.Height != l.Height || a.Bytes != l.Bytes {
			return errors.New("Site Logo index does not match decoded bytes")
		}
		return nil
	})
	if err != nil {
		return b, err
	}

	associated := map[string][]key{}
	for _, k := range order {
		row := rows[k.String()]
		var parent key
		switch k.Table {
		case groupMembers:
			parent = makeKey(sites, field[int64](row.raw, "site_id"))
		case tagMembers:
			parent = makeKey(games, k.first())
		case manualMembers, rules, exclusions:
			parent = makeKey(collections, k.first())
		default:
			continue
		}
		if r := review[k.String()]; r != nil && r.relation.Ignore {
			b.issue(k, "relation_excluded_by_decision")
			continue
		}
		if !k.valid() || !parent.valid() || rows[parent.String()].key.Table == "" {
			b.set(k, "deferred")
			b.issue(k, "missing_relation_parent")
			continue
		}
		associated[parent.String()] = append(associated[parent.String()], k)
	}

	// Tags depend on their categories. Every relation phase runs after all
	// identities, so a collection can reference Game IDs created in this plan.
	for _, table := range []string{sites, games, categories, tags, collections} {
		for _, k := range order {
			if k.Table != table || !k.valid() {
				continue
			}
			row := rows[k.String()]
			r := review[k.String()]
			if r == nil {
				r = &reviewed{}
			}
			it := intent{Key: k, Link: r.link, Alias: r.alias, LinkProof: r.linkProof, DecisionRefs: r.refs}
			it.Editorial.Localizations = mapLoc(&b, row)
			if table == tags || table == categories {
				it.Code = field[string](row.candidate, "legacy_code")
				if r.code != "" {
					it.Code = r.code
				} else if slices.Contains(b.Rows[k.String()].Issues, "duplicate_code") {
					it.IdentityIssue = "tag_code_conflict"
				}
				if len(it.Code) > 64 || !codePattern.MatchString(it.Code) {
					it.IdentityIssue = "tag_code_invalid"
				}
				it.Sort = field[int32](row.raw, "sort_order")
				if stamp := field[string](row.candidate, "archived_at_utc"); stamp != "" {
					it.ArchivedAt, _ = time.Parse(time.RFC3339Nano, stamp)
				} else if v := string(row.raw["archived_at"]); v != "" && v != "null" {
					it.IdentityIssue = "tag_archive_evidence_invalid"
				}
				if table == tags {
					it.Category = makeKey(categories, field[int64](row.raw, "category_id"))
				}
			} else {
				slug := r.slug
				if slug == "" && table == collections {
					slug = field[string](row.candidate, "legacy_code")
				}
				it.Editorial.Slug = slug
				if e, err := catalog.NormalizeEditorial(it.Editorial); err == nil {
					it.Editorial = e
				} else if r.slug != "" {
					return b, errors.New("invalid approved slug")
				} else {
					it.IdentityIssue = "slug_required"
				}
			}
			switch table {
			case sites:
				it.Adult = field[string](row.candidate, "adult_evidence") == "legacy_adult"
				it.Archive = field[bool](row.candidate, "legacy_deleted")
				it.SiteHintURL = field[string](row.candidate, "proposed_url")
				if r.addresses != nil {
					if err = addAddresses(&it, r.addresses, rows); err != nil {
						return b, err
					}
				}
				if r.taxonomy != nil {
					it.Taxonomy = *r.taxonomy
				} else {
					projectGroups(&b, &it, associated[k.String()], rows, review)
				}
				logo := logos[k.first()]
				if logo.Status == "ready" && r.logo == "" && r.link == nil {
					it.LogoIssue = "logo_approval_pending"
				}
				if logo.Status != "ready" && logo.SourceKey != "" && r.link == nil {
					it.LogoIssue = "logo_unavailable_in_seed"
				}
				if r.logo != "" {
					if logo.Status != "ready" || r.logo != logo.SHA {
						return b, errors.New("approved logo does not identify a ready Site Logo")
					}
					it.Logo = r.logo
				}
			case games:
				it.Adult = field[string](row.candidate, "adult_evidence") == "legacy_adult"
				it.Sources, err = chooseSources(row, sources[k.first()], r.sources)
				if err != nil {
					if r.sources != nil {
						return b, err
					}
					it.IdentityIssue = "steam_identity_needs_decision"
				}
				projectRelations(&it, associated[k.String()], rows, review)

			case collections:
				it.Archive = field[string](row.raw, "status") == "archived" && field[string](row.candidate, "archived_at_utc") != ""
				projectRelations(&it, associated[k.String()], rows, review)
				if field[string](row.raw, "status") == "archived" && !it.Archive && r.link == nil {
					it.LifecycleIssue = "collection_archive_evidence_unresolved"
				}
			}
			if r.publish != nil {
				if it.Adult || it.Archive || it.LifecycleIssue != "" || len(it.Editorial.Localizations) == 0 ||
					table == sites && !slices.ContainsFunc(it.Addresses, func(a catalog.Address) bool { return a.Kind == "primary" }) {
					return b, errors.New("publication approval contradicts seed or required presentation")
				}
				it.Publish, it.PublishProof = true, r.publishProof
			}
			if r.link != nil {
				it.IdentityIssue = ""
				it.Relations = nil
				it.RelationIssue = ""
				it.GroupRows = nil
			}
			b.Items = append(b.Items, it)
		}
	}
	// No source can be awarded to a winner merely because its row sorts first.
	claims := map[[2]string][]int{}
	for i, it := range b.Items {
		if it.Key.Table != games || it.Link != nil {
			continue
		}
		for _, s := range it.Sources {
			claim := [2]string{s.PlatformCode, s.ExternalKey}
			claims[claim] = append(claims[claim], i)
		}
	}
	for _, claimants := range claims {
		if len(claimants) > 1 {
			for _, i := range claimants {
				b.Items[i].IdentityIssue = "duplicate_planned_source"
			}
		}
	}

	codes := map[[2]string][]int{}
	for i, it := range b.Items {
		if (it.Key.Table == tags || it.Key.Table == categories) && it.Link == nil && it.IdentityIssue == "" {
			code := [2]string{it.Key.Table, it.Code}
			codes[code] = append(codes[code], i)
		}
	}
	for _, claimants := range codes {
		if len(claimants) > 1 {
			for _, i := range claimants {
				b.Items[i].IdentityIssue = "planned_tag_code_conflict"
			}
		}
	}
	return b, nil
}

func (b *blueprint) set(k key, disposition string) {
	row := b.Rows[k.String()]
	row.Disposition = disposition
	b.Rows[k.String()] = row
}

func addAddresses(it *intent, dec *addressValue, rows map[string]seedRow) error {
	for i, k := range append([]*key{dec.Primary}, pointerKeys(dec.Alternates)...) {
		if k == nil {
			continue
		}
		if !slices.Contains([]string{"gfn_collector_domain", "gfn_site_primary_target_periods", "gfn_target_tracking_periods"}, k.Table) {
			return errors.New("address decision does not reference an address candidate")
		}
		row := rows[k.String()]
		u := field[string](row.candidate, "url")
		if !k.valid() || u == "" || !field[bool](row.candidate, "open_at_snapshot") || field[int64](row.raw, "site_id") != it.Key.first() {
			return errors.New("address decision references an unowned, closed or invalid candidate")
		}
		role := "alternate"
		if i == 0 {
			role = "primary"
		}
		it.AddressRows = append(it.AddressRows, *k)
		it.Addresses = append(it.Addresses, catalog.Address{Kind: role, URL: u})
	}
	addresses, err := catalog.NormalizeSiteAddresses(it.Addresses)
	if err != nil {
		return errors.New("approved addresses duplicate or violate the Catalog address rules")
	}
	it.Addresses = addresses
	return nil
}

func pointerKeys(keys []key) []*key {
	out := make([]*key, 0, len(keys))
	for _, k := range keys {
		out = append(out, &k)
	}
	return out
}

func projectGroups(b *blueprint, it *intent, keys []key, rows map[string]seedRow, review map[string]*reviewed) {
	groupID := func(k key) int64 { return field[int64](rows[k.String()].raw, "group_id") }
	groupRow := func(k key) seedRow { return rows[makeKey(groups, groupID(k)).String()] }
	slices.SortFunc(keys, func(a, b key) int {
		byPriority := cmp.Compare(field[int64](groupRow(a).candidate, "priority"), field[int64](groupRow(b).candidate, "priority"))
		if byPriority != 0 {
			return byPriority
		}
		return cmp.Compare(groupID(a), groupID(b))
	})
	seen := map[int64]bool{}
	for _, k := range keys {
		group := groupRow(k)
		if group.key.Table == "" {
			b.set(k, "deferred")
			b.issue(k, "group_missing")
			continue
		}
		if seen[groupID(k)] {
			b.issue(k, "duplicate_group_evidence")
			continue
		}
		seen[groupID(k)] = true
		r := review[group.key.String()]
		if r == nil || r.group == nil {
			continue
		}
		used := false
		if c := r.group.Category; c != "" && c != it.Taxonomy.Primary && !slices.Contains(it.Taxonomy.Secondary, c) {
			switch {
			case it.Taxonomy.Primary == "":
				it.Taxonomy.Primary = c
				used = true
			case len(it.Taxonomy.Secondary) < 2:
				it.Taxonomy.Secondary = append(it.Taxonomy.Secondary, c)
				used = true
			default:
				b.issue(k, "category_slot_overflow")
			}
		}
		for _, t := range r.group.Tags {
			if slices.Contains(it.Taxonomy.Tags, t) {
				continue
			}
			if len(it.Taxonomy.Tags) == 8 {
				b.issue(k, "navigation_tag_limit")
				continue
			}
			it.Taxonomy.Tags = append(it.Taxonomy.Tags, t)
			used = true
		}
		if used {
			it.GroupRows = append(it.GroupRows, k)
		}
	}
}

func chooseSources(row seedRow, candidate legacyseed.MappingCandidate, approved *[]catalog.Source) ([]catalog.Source, error) {
	options := map[[2]string]catalog.Source{}
	var automatic *catalog.Source
	add := func(platform, key string) (catalog.Source, error) {
		s, err := collection.NormalizeIntakeSource(platform, key)
		if err == nil {
			options[[2]string{s.PlatformCode, s.ExternalKey}] = s
		}
		return s, err
	}
	if candidate.LegacyAppID > 0 && candidate.LegacyAppID <= 4294967295 {
		s, err := add("steam", strconv.FormatInt(candidate.LegacyAppID, 10))
		if err == nil && field[int64](row.candidate, "steam_appid") != 0 {
			automatic = &s
		}
	}
	requiresChoice := candidate.State == "conflict"
	for _, l := range candidate.PlatformLinks {
		u, err := url.Parse(l.URL)
		if err != nil {
			continue
		}
		switch l.Platform {
		case "steam":
			_, _ = add("steam", strings.TrimPrefix(u.Path, "/app/"))
			if l.Relation != "legacy_appid_agrees" {
				requiresChoice = true
			}
		case "itch":
			_, _ = add("itch", strings.TrimSuffix(u.Hostname(), ".itch.io")+u.Path)
		}
	}
	if approved == nil {
		switch {
		case requiresChoice:
			return nil, errors.New("ambiguous Steam source")
		case automatic != nil:
			return []catalog.Source{*automatic}, nil
		default:
			return nil, nil
		}
	}
	seen := map[[2]string]bool{}
	var selected []catalog.Source
	for _, s := range *approved {
		normalized, err := collection.NormalizeIntakeSource(s.PlatformCode, s.ExternalKey)
		sig := [2]string{normalized.PlatformCode, normalized.ExternalKey}
		if err != nil || options[sig] != normalized || seen[sig] || s.SourceURL != "" && s.SourceURL != normalized.SourceURL {
			return nil, errors.New("approved GameSource is duplicate, invalid or absent from seed evidence")
		}
		seen[sig] = true
		selected = append(selected, normalized)
	}
	slices.SortFunc(selected, func(a, b catalog.Source) int {
		if n := strings.Compare(a.PlatformCode, b.PlatformCode); n != 0 {
			return n
		}
		return strings.Compare(a.ExternalKey, b.ExternalKey)
	})
	return selected, nil
}

func projectRelations(it *intent, keys []key, rows map[string]seedRow, review map[string]*reviewed) {
	var manual, other []relation
	positions := map[int]bool{}
	roles := map[string]bool{}
	var explicit int
	for _, k := range keys {
		row := rows[k.String()]
		r := review[k.String()]
		rel := relation{Key: k}
		switch k.Table {
		case tagMembers:
			rel.Reference = makeKey(tags, k.IDs[1])
			rel.Role = field[string](row.candidate, "role")
			if r != nil {
				rel.Role = r.relation.Role
			}
			if !validRole(rel.Role) || rel.Role != "normal" && roles[rel.Role] {
				it.RelationIssue = "invalid_or_duplicate_tag_role"
			}
			roles[rel.Role] = true
		case manualMembers:
			rel.Reference = makeKey(games, k.IDs[1])
			rel.Position = field[int](row.candidate, "export_order")
			if r != nil {
				explicit++
				rel.Position = r.relation.Position
			}
			if rel.Position < 1 || positions[rel.Position] {
				it.RelationIssue = "invalid_or_duplicate_member_position"
			}
			positions[rel.Position] = true
		case rules:
			rel.Reference = makeKey(tags, k.IDs[1])
		case exclusions:
			rel.Reference = makeKey(games, k.IDs[1])
		}
		if rows[rel.Reference.String()].key.Table == "" {
			it.RelationIssue = "orphan_relation"
		}
		if k.Table == manualMembers {
			manual = append(manual, rel)
		} else {
			other = append(other, rel)
		}
	}
	if len(manual) > 512 {
		it.RelationIssue = "collection_capacity_exceeded"
	}
	if explicit != 0 && explicit != len(manual) {
		it.RelationIssue = "incomplete_manual_order"
	}
	slices.SortFunc(manual, func(a, b relation) int {
		if n := cmp.Compare(a.Position, b.Position); n != 0 {
			return n
		}
		return cmp.Compare(a.Reference.first(), b.Reference.first())
	})
	it.Relations = append(manual, other...)
}
