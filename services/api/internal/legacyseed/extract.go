package legacyseed

import (
	"context"
	"encoding/json/v2"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

const issuesFile = "reports/issues.ndjson"
const mappingsFile = "reports/mapping-candidates.ndjson"
const logoFile = "assets/site-logo-index.ndjson"
const exclusionsFile = "reports/excluded-release-rows.ndjson"

type Result struct {
	Status        string `json:"status"`
	ContentSHA    string `json:"content_sha256,omitempty"`
	AssetComplete bool   `json:"asset_complete"`
}

type issueRow struct {
	Source source `json:"source"`
	Code   string `json:"code"`
}

type mappingRow struct {
	Source        source         `json:"source"`
	Kind          string         `json:"kind"`
	State         string         `json:"state"`
	LegacyAppID   int64          `json:"legacy_appid,omitempty"`
	PlatformLinks []platformLink `json:"platform_link_candidates,omitempty"`
}

func mapping(r record) (mappingRow, bool) {
	switch r.Source.Table {
	case "gfn_site_group":
		return mappingRow{Source: r.Source, Kind: "site_classification", State: "unmapped"}, true
	case "gfg_game":
		id := r.Raw.number("appid")
		state := "steam_identity_candidate"
		switch {
		case slices.Contains(r.Issues, "duplicate_appid"):
			state = "conflict"
		case id <= 0 || id > 4294967295:
			state = "unresolved"
		}
		return mappingRow{Source: r.Source, Kind: "game_source_identity", State: state, LegacyAppID: id, PlatformLinks: platformCandidates(r, id)}, true
	}
	return mappingRow{}, false
}

type platformLink struct {
	Platform string `json:"platform"`
	URL      string `json:"url"`
	Relation string `json:"relation"`
}

func platformCandidates(r record, steamID int64) []platformLink {
	links, _ := r.Candidate["links"].([]oldLink)
	var out []platformLink
	for _, link := range links {
		u, err := url.Parse(link.Value)
		if err != nil || u.Scheme != "https" || u.Port() != "" {
			continue
		}
		parts := strings.Split(strings.TrimPrefix(u.Path, "/"), "/")
		var item platformLink
		switch {
		case u.Host == "store.steampowered.com" && len(parts) >= 2 && parts[0] == "app":
			id, err := strconv.ParseUint(parts[1], 10, 32)
			if err != nil || id == 0 || strconv.FormatUint(id, 10) != parts[1] {
				continue
			}
			state := "legacy_link_only"
			if steamID > 0 {
				state = "legacy_appid_agrees"
				if int64(id) != steamID {
					state = "legacy_appid_conflicts"
				}
			}
			item = platformLink{"steam", "https://store.steampowered.com/app/" + parts[1], state}
		case strings.HasSuffix(u.Host, ".itch.io") && len(strings.Split(u.Host, ".")) == 3 && len(parts) == 1 && businessCode.MatchString(parts[0]):
			item = platformLink{"itch", u.String(), "legacy_link_only"}
		default:
			continue
		}
		if !slices.Contains(out, item) {
			out = append(out, item)
		}
	}
	return out
}

func Run(ctx context.Context, mode string, c Config) (Result, error) {
	if !c.Limits.bound() {
		return Result{Status: "incomplete"}, errors.New("invalid D5-A resource limits")
	}
	ctx, cancel := context.WithTimeout(ctx, time.Duration(c.Limits.RunSeconds)*time.Second)
	defer cancel()
	switch mode {
	case "verify":
		return verify(ctx, c)
	case "inspect", "extract":
	default:
		return Result{}, errors.New("expected inspect, extract or verify")
	}
	name := c.Snapshot
	inspect := mode == "inspect"
	if inspect {
		name += ".inspect"
	}
	result, err := stage(c, name, func(b *bundle) (Result, error) {
		s := summary{Version: formatVersion, Tables: map[string]tableCount{}, Issues: map[string]int{}, Logos: map[string]int{}}
		if err := extract(ctx, c, b, inspect, &s); err != nil {
			return Result{}, err
		}
		s.ExtractComplete = !inspect
		s.AssetComplete = !inspect && s.Logos["ready"]+s.Logos["missing_icon"] == s.Tables["gfn_site"].Source
		s.CleanSeedReady = !inspect
		s.Reasons = readinessReasons(s, inspect)
		if err := b.report(s); err != nil {
			return Result{}, err
		}
		if inspect {
			return Result{Status: "inspected", AssetComplete: s.AssetComplete}, nil
		}
		content, err := b.seal(c, s)
		if err != nil {
			return Result{}, err
		}
		if _, err := verifyBundle(ctx, b.root, c); err != nil {
			return Result{}, err
		}
		return Result{"ready_for_D5_B", content, s.AssetComplete}, nil
	})
	if err != nil {
		// Failed runs only publish a fixed, redacted diagnostic. The error is
		// constructed by the extractor from static operation/descriptor names.
		failureConfig := c
		failureConfig.Limits.PackageBytes = maxMetadata
		_, _ = stage(failureConfig, name+".failed", func(b *bundle) (Result, error) {
			raw, _ := marshal(struct {
				ExtractComplete bool   `json:"extract_complete"`
				AssetComplete   bool   `json:"asset_complete"`
				CleanSeedReady  bool   `json:"clean_seed_ready"`
				Reason          string `json:"reason"`
			}{Reason: err.Error()})
			return Result{}, b.blob("reports/summary.json", append(raw, '\n'))
		})
		return Result{Status: "incomplete"}, err
	}
	return result, nil
}

func readinessReasons(s summary, inspect bool) []string {
	r := []string{"GFN_GFG_rows_reconciled"}
	if inspect {
		r = append(r, "inspect_does_not_create_a_seed", "assets_not_inspected")
	} else if !s.AssetComplete {
		r = append(r, "some_referenced_logos_are_not_packaged")
	}
	if len(s.Issues) > 0 {
		r = append(r, "quality_issues_retained_for_mapping")
	}
	if s.GFA != "aggregate_only" {
		r = append(r, "GFA_optional_aggregate_unavailable")
	}
	return r
}

func extract(ctx context.Context, c Config, b *bundle, inspect bool, s *summary) error {
	var dest packageWriter = discardPackage{}
	if !inspect {
		dest = b
		for _, t := range tables {
			if _, err := b.file(t.path()); err != nil {
				return err
			}
		}
		for _, path := range []string{issuesFile, mappingsFile, logoFile, exclusionsFile} {
			if _, err := b.file(path); err != nil {
				return err
			}
		}
	}
	logos := logoReader{inspect: inspect, limit: c.Limits.LogoObjects, cache: map[string]logo{}, seen: map[string]bool{}}
	if !inspect && c.Logos != "" {
		root, err := privateRoot(c.Logos, false)
		if err != nil {
			return err
		}
		defer root.Close()
		logos.root = root
	}
	gfn, gfnTX, err := openSnapshot(ctx, c.GFN)
	if err != nil {
		return fmt.Errorf("gfn: %w", err)
	}
	defer closeConn(gfn)
	gfg, gfgTX, err := openSnapshot(ctx, c.GFG)
	if err != nil {
		return fmt.Errorf("gfg: %w", err)
	}
	defer closeConn(gfg)
	databases := map[string]pgx.Tx{"gfn": gfnTX, "gfg": gfgTX}
	for _, t := range tables {
		if err := checkTable(ctx, databases[t.db], t); err != nil {
			return err
		}
		count, err := sourceCount(ctx, databases[t.db], t)
		if err != nil {
			return err
		}
		s.Tables[t.name] = count
	}
	total := 0
	for _, n := range s.Tables {
		total += n.Source
	}
	if total > c.Limits.SourceRows {
		return fmt.Errorf("source tables exceed configured %d rows", c.Limits.SourceRows)
	}
	if !inspect && c.SVGRenderer != "" {
		info, err := os.Stat(c.SVGRenderer)
		if !filepath.IsAbs(c.SVGRenderer) || err != nil || !info.Mode().IsRegular() {
			return errors.New("logo_svg_renderer requires an existing absolute executable path")
		}
		logos.renderer = c.SVGRenderer
		logos.shared, err = sharedLogoDigests(ctx, gfnTX)
		if err != nil {
			return errors.New("cannot inventory Site Logo references")
		}
	}
	a, err := readAddresses(ctx, gfnTX)
	if err != nil {
		return err
	}
	parents := relationIndex{}
	for _, t := range tables {
		n := s.Tables[t.name]
		var last source
		err := scan(ctx, databases[t.db], t, func(raw values) error {
			r := normalize(t, raw)
			if last.Table != "" && sourceOrder(last, r.Source) >= 0 {
				return errors.New("duplicate or unordered source key")
			}
			last = r.Source
			a.apply(&r)
			for _, code := range parents.violations(r) {
				r.issue(code)
			}
			parents.remember(r)
			if t.name == "gfn_site" {
				obj, err := logos.load(ctx, &r, dest)
				if err != nil {
					return err
				}
				s.Logos[obj.Status]++
				if err := dest.row(logoFile, obj); err != nil {
					return err
				}
			}
			r.classify()
			if r.Status == "usable" {
				n.Usable++
			} else {
				n.Unresolved++
			}
			for _, code := range r.Issues {
				s.Issues[code]++
				if err := dest.row(issuesFile, issueRow{r.Source, code}); err != nil {
					return err
				}
			}
			if m, ok := mapping(r); ok {
				if err := dest.row(mappingsFile, m); err != nil {
					return err
				}
			}
			return dest.row(t.path(), r)
		})
		if err != nil {
			return err
		}
		if t.filter != "" && !inspect {
			if err := excludedRows(ctx, databases[t.db], t, dest, n.Excluded); err != nil {
				return err
			}
		}
		if n.Source != n.Usable+n.Unresolved+n.Excluded {
			return fmt.Errorf("%s: source row count mismatch", t.name)
		}
		s.Tables[t.name] = n
	}
	for name, tx := range databases {
		if tx.Commit(ctx) != nil {
			return fmt.Errorf("%s: cannot close read-only source snapshot", name)
		}
	}
	s.UniqueLogos = len(logos.seen)
	s.GFA, s.Ideas = readIdeas(ctx, c.GFA)
	if ctx.Err() != nil {
		return ctx.Err()
	}
	return nil
}

func sharedLogoDigests(ctx context.Context, tx pgx.Tx) (map[string]int, error) {
	rows, err := tx.Query(ctx, "SELECT id, icon FROM public.gfn_site WHERE icon IS NOT NULL")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	counts := map[string]int{}
	for rows.Next() {
		var siteID int64
		var key string
		if err := rows.Scan(&siteID, &key); err != nil {
			return nil, err
		}
		if logoKeyPattern.MatchString(key) && logoBelongsTo(key, siteID) {
			counts[logoSourceDigest(key)]++
		}
	}
	return counts, rows.Err()
}

func scan(ctx context.Context, tx pgx.Tx, t table, receive func(values) error) error {
	if t.filter == "false" {
		return nil
	}
	if t.name != "gfn_site" {
		err := scanRows(ctx, tx, t, t.query(), receive)
		return err
	}
	// Release each FETCH response before processing Logo bytes. A slow local
	// codec must not hold the server's statement open against TCP backpressure.
	if _, err := tx.Exec(ctx, "DECLARE legacy_sites NO SCROLL CURSOR FOR "+t.query()); err != nil {
		return errors.New("gfn_site: cannot open read-only source cursor")
	}
	const batch = 16
	for {
		chunk := make([]values, 0, batch)
		err := scanRows(ctx, tx, t, "FETCH FORWARD 16 FROM legacy_sites", func(v values) error {
			chunk = append(chunk, v)
			return nil
		})
		if err != nil {
			return err
		}
		for _, v := range chunk {
			if err := receive(v); err != nil {
				return fmt.Errorf("%s: %w", t.name, err)
			}
		}
		if len(chunk) < batch {
			return nil
		}
	}
}

func scanRows(ctx context.Context, tx pgx.Tx, t table, query string, receive func(values) error) error {
	rows, err := tx.Query(ctx, query)
	if err != nil {
		return fmt.Errorf("%s: source read failed", t.name)
	}
	defer rows.Close()
	for rows.Next() {
		var body []byte
		var v values
		if rows.Scan(&body) != nil || len(body) > maxLine || json.Unmarshal(body, &v) != nil {
			return fmt.Errorf("%s: invalid or oversized source row", t.name)
		}
		if err := receive(v); err != nil {
			return fmt.Errorf("%s: %w", t.name, err)
		}
	}
	if rows.Err() != nil {
		return fmt.Errorf("%s: source read failed", t.name)
	}
	return nil
}

func sourceCount(ctx context.Context, tx pgx.Tx, t table) (tableCount, error) {
	var out tableCount
	query := "SELECT count(*) FROM (SELECT 1 FROM public." + t.name + " LIMIT 250001) AS bounded"
	if tx.QueryRow(ctx, query).Scan(&out.Source) != nil {
		return out, fmt.Errorf("%s: source count failed", t.name)
	}
	if t.filter != "" {
		if tx.QueryRow(ctx, "SELECT count(*) FROM public."+t.name+" WHERE ("+t.filter+") IS NOT TRUE").Scan(&out.Excluded) != nil {
			return out, fmt.Errorf("%s: excluded row count failed", t.name)
		}
		out.Exclusion = "non_editorial_release_evidence"
	}
	return out, nil
}

func excludedRows(ctx context.Context, tx pgx.Tx, t table, dest packageWriter, expected int) error {
	rows, err := tx.Query(ctx, "SELECT game_id FROM public."+t.name+" WHERE ("+t.filter+") IS NOT TRUE ORDER BY game_id")
	if err != nil {
		return fmt.Errorf("%s: cannot read excluded source identities", t.name)
	}
	defer rows.Close()
	count := 0
	var prev int64
	for rows.Next() {
		var id int64
		if rows.Scan(&id) != nil || (count > 0 && id <= prev) {
			return fmt.Errorf("%s: invalid excluded source identity", t.name)
		}
		count++
		prev = id
		if err := dest.row(exclusionsFile, issueRow{source{t.db, t.name, id, []int64{id}}, "non_editorial_release_evidence"}); err != nil {
			return err
		}
	}
	if rows.Err() != nil || count != expected {
		return fmt.Errorf("%s: excluded source count mismatch", t.name)
	}
	return nil
}

func readIdeas(ctx context.Context, c Database) (string, map[string]int64) {
	conn, tx, err := openSnapshot(ctx, c)
	if err != nil {
		return "unavailable", nil
	}
	defer closeConn(conn)
	if !canReadIdeas(ctx, tx) {
		return "unsupported", nil
	}
	out := map[string]int64{}
	rows, err := tx.Query(ctx, `SELECT CASE WHEN kind IN ('game','site','other') THEN kind ELSE 'unknown' END, count(*)
		FROM (SELECT kind FROM public.gfa_content_idea LIMIT 250001) bounded GROUP BY 1`)
	if err != nil {
		return "unavailable", nil
	}
	defer rows.Close()
	for rows.Next() {
		var kind string
		var count int64
		if rows.Scan(&kind, &count) != nil {
			return "unavailable", nil
		}
		out[kind] = count
		out["total"] += count
	}
	if rows.Err() != nil || tx.Commit(ctx) != nil {
		return "unavailable", nil
	}
	if out["total"] > rowLimit {
		return "resource_limit", nil
	}
	return "aggregate_only", out
}

func validStatus(s string) bool {
	return slices.Contains([]string{"usable", "invalid", "archived", "conflict", "needs_mapping"}, s)
}

func canonicalLine(body []byte) bool {
	return len(body) > 0 && body[len(body)-1] == '\n' && !strings.ContainsRune(string(body[:len(body)-1]), '\n')
}
