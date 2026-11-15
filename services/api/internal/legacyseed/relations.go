package legacyseed

import (
	"context"
	"fmt"
	"net/url"
	"slices"
	"strings"

	"github.com/jackc/pgx/v5"
)

type reference struct {
	field, parent string
	optional      bool
}

var references = map[string][]reference{
	"gfn_site_group_map":              {{"site_id", "gfn_site", false}, {"group_id", "gfn_site_group", false}},
	"gfn_collector_domain":            {{"site_id", "gfn_site", true}},
	"gfn_target_tracking_periods":     {{"site_id", "gfn_site", false}, {"collector_domain_id", "gfn_collector_domain", true}},
	"gfn_site_primary_target_periods": {{"site_id", "gfn_site", false}, {"target_tracking_period_id", "gfn_target_tracking_periods", false}},
	"gfn_featured_site":               {{"site_id", "gfn_site", false}},
	"gfg_tag":                         {{"category_id", "gfg_tag_category", false}},
	"gfg_game_tag":                    {{"game_id", "gfg_game", false}, {"tag_id", "gfg_tag", false}},
	"gfg_game_collection_item":        {{"collection_id", "gfg_game_collection", false}, {"game_id", "gfg_game", false}},
	"gfg_game_collection_tag":         {{"collection_id", "gfg_game_collection", false}, {"tag_id", "gfg_tag", false}},
	"gfg_game_collection_exclusion":   {{"collection_id", "gfg_game_collection", false}, {"game_id", "gfg_game", false}},
	"gfg_game_collection_home_slot":   {{"collection_id", "gfg_game_collection", false}},
	"gfg_game_first_available":        {{"game_id", "gfg_game", false}},
}

type parentState struct{ archived bool }

type relationIndex map[string]map[int64]parentState

func (x relationIndex) remember(r record) {
	switch r.Source.Table {
	case "gfn_site", "gfn_site_group", "gfn_collector_domain", "gfn_target_tracking_periods", "gfg_game", "gfg_tag_category", "gfg_tag", "gfg_game_collection":
		if x[r.Source.Table] == nil {
			x[r.Source.Table] = map[int64]parentState{}
		}
		state := parentState{archived: r.Raw.yes("deleted") || !r.Raw.null("archived_at") || r.Raw.str("status") == "archived"}
		if r.Source.Table == "gfg_tag" {
			state.archived = state.archived || x["gfg_tag_category"][r.Raw.number("category_id")].archived
		}
		x[r.Source.Table][r.Source.LegacyID] = state
	}
}

func (x relationIndex) violations(r record) []string {
	var out []string
	for _, ref := range references[r.Source.Table] {
		if r.Raw.null(ref.field) && ref.optional {
			continue
		}
		state, ok := x[ref.parent][r.Raw.number(ref.field)]
		switch {
		case !ok:
			out = append(out, "orphan_"+ref.field)
		case state.archived:
			out = append(out, "archived_reference_"+ref.field)
		}
	}
	return out
}

type addressEvidence struct {
	site, collector               int64
	url, host, from, until, basis string
	issues                        []string
	active                        bool
}

type addressChoice struct {
	url, basis string
	rank       int
	conflict   bool
}

type addressIndex struct {
	records          map[string]map[int64]addressEvidence
	choices          map[int64]addressChoice
	owner            map[string]int64
	shared           map[string]bool
	primaryCount     map[int64]int
	retainedURLBytes int
}

func (a *addressIndex) offer(site int64, url, basis string, rank int) {
	if site <= 0 || url == "" {
		return
	}
	old := a.choices[site]
	if rank > old.rank {
		a.choices[site] = addressChoice{url: url, basis: basis, rank: rank}
	} else if old.rank == rank && old.url != url {
		old.conflict = true
		a.choices[site] = old
	}
	host := domainOf(url)
	if prev, exists := a.owner[host]; exists && prev != site {
		a.shared[host] = true
	} else {
		a.owner[host] = site
	}
}

func domainOf(value string) string {
	u, err := url.Parse(value)
	if err != nil {
		return ""
	}
	return u.Hostname()
}

func (a *addressIndex) add(name string, v values) error {
	e := addressEvidence{site: v.number("site_id")}
	switch name {
	case "gfn_collector_domain":
		e.host = strings.TrimSpace(v.str("prefix") + v.str("name"))
		if len(e.host) > 260 {
			e.host = ""
		}
		scheme := ""
		switch v.str("tls") {
		case "1":
			scheme = "https"
		case "0":
			scheme = "http"
		}
		if scheme != "" && !strings.ContainsAny(e.host, "/?#@\\") {
			e.url = cleanURL(scheme + "://" + e.host)
		}
		e.basis, e.active = "legacy_config", !v.yes("deleted")
		if v.null("site_id") {
			e.issues = append(e.issues, "address_unassigned")
		}
		if e.active {
			a.offer(e.site, e.url, e.basis, 1)
		}
	case "gfn_target_tracking_periods":
		e.host, e.collector = v.str("target"), v.number("collector_domain_id")
		e.from, e.until = normalizedTime(v.str("tracked_from"), "z"), normalizedTime(v.str("tracked_until"), "z")
		valid := e.checkPeriod(v.null("tracked_until"))
		e.active = valid && v.null("tracked_until")
		e.basis = v.str("tracking_basis")
		if e.basis != "explicit" && e.basis != "legacy_observed" {
			e.issues = append(e.issues, "unknown_tracking_basis")
			e.active = false
		}
		e.url = cleanURL(e.host)
		if c, ok := a.records["gfn_collector_domain"][e.collector]; ok {
			if c.site != e.site {
				e.issues = append(e.issues, "address_ownership_conflict")
			} else if e.url == "" && strings.EqualFold(c.host, e.host) {
				// A current TLS flag only establishes a scheme for the open,
				// same-host period. Closed periods retain their original target.
				if e.active && c.active {
					e.url = c.url
				}
			}
		}
		if e.active && len(e.issues) == 0 {
			a.offer(e.site, e.url, "active_tracking_period", 2)
		}
		if e.url == "" && !strings.Contains(e.host, "://") && cleanURL("https://"+e.host) != "" {
			e.issues = append(e.issues, "address_scheme_unknown")
		}
		e.host = ""
	case "gfn_site_primary_target_periods":
		e.from, e.until = normalizedTime(v.str("effective_from"), "z"), normalizedTime(v.str("effective_until"), "z")
		valid := e.checkPeriod(v.null("effective_until"))
		e.active = valid && v.null("effective_until")
		e.basis = v.str("basis")
		ranks := map[string]int{"legacy_backfill": 3, "deterministic_fallback": 4, "single_target_inferred": 5, "explicit": 6}
		rank := ranks[e.basis]
		t, ok := a.records["gfn_target_tracking_periods"][v.number("target_tracking_period_id")]
		switch {
		case !ok:
			e.issues = append(e.issues, "missing_primary_tracking_period")
		case t.site != e.site:
			e.issues = append(e.issues, "address_ownership_conflict")
		case v.str("effective_from") == "" || t.from == "" || earlier(e.from, t.from) ||
			(t.until != "" && (e.until == "" || earlier(t.until, e.until))):
			e.issues = append(e.issues, "primary_outside_tracking_period")
		default:
			e.url = t.url
		}
		if rank == 0 {
			e.issues = append(e.issues, "unknown_primary_basis")
		}
		if e.active {
			a.primaryCount[e.site]++
		}
		if e.active && t.active && len(e.issues) == 0 {
			a.offer(e.site, e.url, e.basis, rank)
		}
	}
	if e.url == "" && !slices.Contains(e.issues, "address_scheme_unknown") {
		e.issues = append(e.issues, "address_invalid")
	}
	a.retainedURLBytes += len(e.url) + len(e.host)
	if a.retainedURLBytes > 32<<20 {
		return fmt.Errorf("address evidence URL data exceeds 32 MiB")
	}
	a.records[name][v.number("id")] = e
	return nil
}

func earlier(a, b string) bool {
	// RFC3339Nano strips trailing fractions, so compare the seconds and
	// normalize their terminators before comparing the fractional suffix.
	return strings.TrimSuffix(a, "Z") < strings.TrimSuffix(b, "Z")
}

func (e *addressEvidence) checkPeriod(noEnd bool) bool {
	if e.from == "" || (!noEnd && (e.until == "" || !earlier(e.from, e.until))) {
		e.issues = append(e.issues, "invalid_address_period")
		return false
	}
	return true
}

func newAddressIndex() *addressIndex {
	return &addressIndex{map[string]map[int64]addressEvidence{
		"gfn_collector_domain": {}, "gfn_target_tracking_periods": {}, "gfn_site_primary_target_periods": {},
	}, map[int64]addressChoice{}, map[string]int64{}, map[string]bool{}, map[int64]int{}, 0}
}

func readAddresses(ctx context.Context, tx pgx.Tx) (*addressIndex, error) {
	a := newAddressIndex()
	total := 0
	for _, s := range tables {
		if s.file != addresses {
			continue
		}
		if err := scan(ctx, tx, s, func(v values) error {
			total++
			if total > rowLimit {
				return fmt.Errorf("address index exceeds %d rows", rowLimit)
			}
			return a.add(s.name, v)
		}); err != nil {
			return nil, err
		}
	}
	return a, nil
}

func (a *addressIndex) apply(r *record) {
	if r.Source.Table == "gfn_site" {
		r.Candidate["proposed_url"] = nil
		c := a.choices[r.Source.LegacyID]
		if a.primaryCount[r.Source.LegacyID] > 1 {
			r.issue("address_ambiguous")
		}
		switch {
		case c.conflict:
			r.issue("address_ambiguous")
		case c.url == "":
			r.issue("address_missing")
		case a.shared[domainOf(c.url)]:
			r.issue("duplicate_address")
		default:
			r.Candidate["proposed_url"] = c.url
			r.Candidate["address_basis"] = c.basis
		}
		return
	}
	if rows, ok := a.records[r.Source.Table]; ok {
		e := rows[r.Source.LegacyID]
		r.Candidate["url"] = nil
		if e.url != "" {
			r.Candidate["url"] = e.url
		}
		r.Candidate["address_basis"] = e.basis
		r.Candidate["open_at_snapshot"] = e.active
		for _, issue := range e.issues {
			r.issue(issue)
		}
		if a.shared[domainOf(e.url)] {
			r.issue("duplicate_address")
		}
	}
}

func sourceOrder(a, b source) int {
	if a.Table != b.Table {
		return tableOrder[a.Table] - tableOrder[b.Table]
	}
	return slices.Compare(a.Key, b.Key)
}

var tableOrder = func() map[string]int {
	out := map[string]int{}
	for i, s := range tables {
		out[s.name] = i
	}
	return out
}()
