package legacyseed

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json/jsontext"
	"encoding/json/v2"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/librecosta/gofurry/services/api/internal/storage"
)

func verify(ctx context.Context, c Config) (Result, error) {
	root, err := privateRoot(filepath.Join(c.Output, c.Snapshot), false)
	if err != nil {
		return Result{Status: "incomplete"}, err
	}
	defer root.Close()
	m, err := verifyBundle(ctx, root, c)
	if err != nil {
		return Result{Status: "incomplete"}, err
	}
	return Result{"ready_for_D5_B", m.ContentSHA, m.Summary.AssetComplete}, nil
}

func verifyBundle(ctx context.Context, root *os.Root, c Config) (manifest, error) {
	var m manifest
	raw, err := readBytes(root, "manifest.json", maxMetadata)
	if err != nil || !canonicalLine(raw) || json.Unmarshal(raw, &m, json.RejectUnknownMembers(true)) != nil ||
		m.Version != formatVersion || m.Snapshot != c.Snapshot || m.BackupSHA != c.BackupSHA ||
		m.LegacyCommit != SourceCommit || m.CreatedAt.IsZero() || m.CreatedAt.Format("Z07:00") != "Z" ||
		m.ContentSHA != sha(checksumLines(m.Files)) || len(m.Files) > logoReadLimit+32 {
		return m, errors.New("invalid package manifest or version")
	}
	files, err := verifyFiles(ctx, root, m, raw)
	if err != nil {
		return m, err
	}
	if err := verifyRecords(ctx, root, m, files); err != nil {
		return m, err
	}
	return m, nil
}

func verifyFiles(ctx context.Context, root *os.Root, m manifest, raw []byte) (map[string]entry, error) {
	required := map[string]bool{issuesFile: true, mappingsFile: true, logoFile: true, exclusionsFile: true, "reports/summary.json": true, "reports/summary.md": true}
	for _, t := range tables {
		required[t.path()] = true
	}
	files := map[string]entry{}
	var total int64
	sumEntries := append(slices.Clone(m.Files), entry{Path: "manifest.json", SHA: sha(raw)})
	slices.SortFunc(sumEntries, func(a, b entry) int { return strings.Compare(a.Path, b.Path) })
	last := ""
	for _, f := range m.Files {
		logo := outputLogoPattern.MatchString(f.Path)
		if f.Path <= last || (!required[f.Path] && !logo) || !digestPattern.MatchString(f.SHA) || f.Bytes < 0 || f.Bytes > maxPackageBytes ||
			f.Rows < 0 || (logo && (f.Bytes > imageOutputLimit || f.SHA != strings.TrimSuffix(filepath.Base(f.Path), ".webp"))) ||
			(!strings.HasSuffix(f.Path, ".ndjson") && f.Rows != 0) {
			return nil, errors.New("unexpected file entry in package")
		}
		last = f.Path
		files[f.Path] = f
		total += f.Bytes
		if total > maxPackageBytes {
			return nil, errors.New("package exceeds 1 GiB")
		}
	}
	for p := range required {
		if files[p].Path == "" {
			return nil, errors.New("required package file is missing")
		}
	}
	sums, err := readBytes(root, "SHA256SUMS", maxMetadata)
	if err != nil || !bytes.Equal(sums, checksumLines(sumEntries)) {
		return nil, errors.New("invalid SHA256SUMS")
	}
	if err := verifyTree(ctx, root, files); err != nil {
		return nil, err
	}
	for _, f := range m.Files {
		if err := verifyFile(ctx, root, f); err != nil {
			return nil, errors.New("package file is missing, linked or has a checksum mismatch")
		}
	}
	return files, nil
}

func verifyTree(ctx context.Context, root *os.Root, files map[string]entry) error {
	dirs := map[string]bool{"content": true, "reports": true, "assets": true, "assets/site-logos": true}
	for _, parent := range []string{".", "content", "reports", "assets", "assets/site-logos"} {
		info, err := root.Lstat(parent)
		if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return errors.New("required package directory is missing or linked")
		}
		f, err := root.Open(parent)
		if err != nil {
			return errors.New("cannot read private package directory")
		}
		err = verifyDirectory(ctx, f, parent, files, dirs)
		f.Close()
		if err != nil {
			return err
		}
	}
	return nil
}

func verifyDirectory(ctx context.Context, f *os.File, parent string, files map[string]entry, dirs map[string]bool) error {
	for {
		items, err := f.ReadDir(128)
		if err == io.EOF {
			return nil
		}
		if err != nil || ctx.Err() != nil {
			return errors.New("cannot enumerate private package")
		}
		for _, item := range items {
			path := filepath.ToSlash(filepath.Join(parent, item.Name()))
			if item.Type()&os.ModeSymlink != 0 {
				return errors.New("linked package entry")
			}
			if item.IsDir() {
				if !dirs[path] {
					return errors.New("unexpected package directory")
				}
				continue
			}
			if !item.Type().IsRegular() || (files[path].Path == "" && path != "manifest.json" && path != "SHA256SUMS") {
				return errors.New("unexpected private package file")
			}
		}
	}
}

type lineReader struct {
	f     *os.File
	r     *bufio.Reader
	count int
}

func openLines(ctx context.Context, root *os.Root, path string) (*lineReader, error) {
	f, err := openPrivateFile(root, path)
	if err != nil {
		return nil, err
	}
	return &lineReader{f: f, r: bufio.NewReaderSize(&contextReader{ctx, f}, maxLine)}, nil
}

func (r *lineReader) next(v any) error {
	raw, err := r.r.ReadSlice('\n')
	if err == io.EOF && len(raw) == 0 {
		return io.EOF
	}
	if err != nil || !canonicalLine(raw) || json.Unmarshal(raw, v, json.RejectUnknownMembers(true)) != nil {
		return errors.New("invalid or oversized NDJSON record")
	}
	r.count++
	return nil
}

func (r *lineReader) expected(v any) error {
	var raw jsontext.Value
	if err := r.next(&raw); err != nil {
		return errors.New("missing derived NDJSON record")
	}
	want, err := marshal(v)
	if err != nil || !bytes.Equal(raw, want) {
		return errors.New("derived NDJSON record does not match content")
	}
	return nil
}

type wireRecord struct {
	Source    source         `json:"source"`
	Status    string         `json:"status"`
	Raw       values         `json:"raw"`
	Candidate jsontext.Value `json:"candidate"`
	Issues    []string       `json:"issues"`
}

func visitRows(ctx context.Context, root *os.Root, f entry, expectedTable table, visit func(wireRecord) error) error {
	r, err := openLines(ctx, root, f.Path)
	if err != nil {
		return err
	}
	defer r.f.Close()
	var last source
	for {
		var w wireRecord
		err := r.next(&w)
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		idx, ok := tableOrder[w.Source.Table]
		if !ok || tables[idx].path() != f.Path || w.Source.Database != tables[idx].db ||
			len(w.Source.Key) != len(strings.Split(tables[idx].keys, ",")) || w.Source.LegacyID != w.Source.Key[0] ||
			(last.Table != "" && sourceOrder(last, w.Source) >= 0) || !validStatus(w.Status) ||
			!slices.IsSorted(w.Issues) || !validRaw(tables[idx], w.Raw) || r.count > rowLimit {
			return errors.New("invalid NDJSON source, keys, order, schema or status")
		}
		last = w.Source
		if w.Source.Table == expectedTable.name {
			if err := visit(w); err != nil {
				return fmt.Errorf("%s: %w", expectedTable.name, err)
			}
		}
	}
	if r.count != f.Rows {
		return errors.New("manifest NDJSON row count differs from file")
	}
	return nil
}

func validRaw(t table, raw values) bool {
	size := 0
	for _, field := range t.fields() {
		name, typ, _ := strings.Cut(field, ":")
		if t.name == "gfn_site" && name == "icon" {
			continue
		}
		size++
		v, ok := raw[name]
		if !ok {
			return false
		}
		kind := v.Kind()
		if kind == 'n' || typ == "j" {
			continue
		}
		switch typ {
		case "i", "h", "n":
			if kind != '0' {
				return false
			}
		case "b":
			if kind != 't' && kind != 'f' {
				return false
			}
		case "s", "z", "l", "d":
			if kind != '"' && string(v) != `{"source_value_oversize": true}` && string(v) != `{"source_value_oversize":true}` {
				return false
			}
		}
	}
	for _, extra := range map[string][]string{
		"gfn_site_group":     {"zh_name_count", "en_name_count"},
		"gfn_site_group_map": {"pair_count"},
		"gfn_featured_site":  {"featured_count"},
		"gfg_game":           {"appid_count", "legacy_adult"}, "gfg_tag": {"code_count"}, "gfg_tag_category": {"code_count"},
		"gfg_game_tag": {"role_count"}, "gfg_game_collection": {"code_count"},
		"gfg_game_collection_item":      {"member_count", "export_order", "explicitly_excluded"},
		"gfg_game_collection_home_slot": {"slot_count"},
	}[t.name] {
		kind := raw[extra].Kind()
		if extra == "legacy_adult" || extra == "explicitly_excluded" {
			if kind != 't' && kind != 'f' {
				return false
			}
		} else if kind != '0' || raw.number(extra) < 1 {
			return false
		}
		size++
	}
	return len(raw) == size
}

func verifyRecords(ctx context.Context, root *os.Root, m manifest, files map[string]entry) error {
	s := m.Summary
	if s.Version != formatVersion || !s.ExtractComplete || !s.CleanSeedReady || len(s.Tables) != len(tables) || !validIdeas(s) {
		return errors.New("invalid package reconciliation summary")
	}
	count := 0
	for _, t := range tables {
		n, ok := s.Tables[t.name]
		if !ok || n.Source < 0 || n.Source > rowLimit || n.Usable < 0 || n.Unresolved < 0 || n.Excluded < 0 ||
			n.Usable > n.Source || n.Unresolved > n.Source || n.Excluded > n.Source || n.Source != n.Usable+n.Unresolved+n.Excluded ||
			(t.filter == "" && (n.Excluded != 0 || n.Exclusion != "")) ||
			(t.filter != "" && n.Exclusion != "non_editorial_release_evidence") || (t.filter == "false" && n.Source != n.Excluded) {
			return errors.New("invalid source table reconciliation")
		}
		count += n.Source
	}
	if count > rowLimit {
		return errors.New("source row limit exceeded")
	}

	address := newAddressIndex()
	for _, t := range tables {
		if t.file != addresses {
			continue
		}
		if err := visitRows(ctx, root, files[t.path()], t, func(w wireRecord) error {
			return address.add(t.name, w.Raw)
		}); err != nil {
			return err
		}
	}
	issues, err := openLines(ctx, root, issuesFile)
	if err != nil {
		return errors.New("invalid issues file")
	}
	defer issues.f.Close()
	mappings, err := openLines(ctx, root, mappingsFile)
	if err != nil {
		return errors.New("invalid mappings file")
	}
	defer mappings.f.Close()
	logos, err := openLines(ctx, root, logoFile)
	if err != nil {
		return errors.New("invalid Logo index")
	}
	defer logos.f.Close()
	exclusions, err := openLines(ctx, root, exclusionsFile)
	if err != nil {
		return errors.New("invalid excluded release index")
	}
	defer exclusions.f.Close()
	parents := relationIndex{}
	actualIssues, actualLogos := map[string]int{}, map[string]int{}
	seen := map[string]logo{}
	for _, t := range tables {
		usable, unresolved := 0, 0
		manualKeys := map[int64]bool{}
		err := visitRows(ctx, root, files[t.path()], t, func(w wireRecord) error {
			r := normalize(t, w.Raw)
			if t.filter != "" {
				if t.filter == "false" || r.Raw.str("source") != "legacy_manual" || string(r.Raw["inferred"]) != "false" {
					return errors.New("non-editorial release evidence in candidate rows")
				}
				manualKeys[r.Source.LegacyID] = true
			}
			address.apply(&r)
			for _, code := range parents.violations(r) {
				r.issue(code)
			}
			parents.remember(r)
			if t.name == "gfn_site" {
				var obj logo
				if logos.next(&obj) != nil || obj.SiteID != r.Source.LegacyID {
					return errors.New("missing Site Logo index record")
				}
				if err := verifyLogo(ctx, root, files, obj, seen, &r); err != nil {
					return err
				}
				actualLogos[obj.Status]++
			}
			r.classify()
			normalized, err := marshal(r.Candidate)
			if err != nil || !bytes.Equal(normalized, w.Candidate) || r.Status != w.Status || !slices.Equal(r.Issues, w.Issues) ||
				sourceOrder(r.Source, w.Source) != 0 {
				return errors.New("record candidates, references or reconciliation differ from source values")
			}
			if r.Status == "usable" {
				usable++
			} else {
				unresolved++
			}
			for _, code := range r.Issues {
				if err := issues.expected(issueRow{r.Source, code}); err != nil {
					return err
				}
				actualIssues[code]++
			}
			if item, ok := mapping(r); ok {
				return mappings.expected(item)
			}
			return nil
		})
		if err != nil {
			return err
		}
		n := s.Tables[t.name]
		if n.Usable != usable || n.Unresolved != unresolved {
			return errors.New("content row reconciliation mismatch")
		}
		var prev int64
		for i := 0; i < n.Excluded; i++ {
			var item issueRow
			if exclusions.next(&item) != nil || item.Source.Database != t.db || item.Source.Table != t.name ||
				len(item.Source.Key) != 1 || item.Source.Key[0] != item.Source.LegacyID ||
				(i > 0 && item.Source.LegacyID <= prev) || manualKeys[item.Source.LegacyID] ||
				item.Code != "non_editorial_release_evidence" {
				return errors.New("excluded release row reconciliation mismatch")
			}
			prev = item.Source.LegacyID
		}
	}
	for _, stream := range []struct {
		reader *lineReader
		path   string
	}{{issues, issuesFile}, {mappings, mappingsFile}, {logos, logoFile}, {exclusions, exclusionsFile}} {
		var extra jsontext.Value
		if stream.reader.count != files[stream.path].Rows || stream.reader.next(&extra) != io.EOF {
			return errors.New("unreconciled index rows")
		}
	}
	logoObjects := 0
	for p := range files {
		if outputLogoPattern.MatchString(p) {
			logoObjects++
		}
	}
	if logoObjects != len(seen) || len(seen) != s.UniqueLogos ||
		s.AssetComplete != (actualLogos["ready"]+actualLogos["missing_icon"] == s.Tables["gfn_site"].Source) {
		return errors.New("Logo object reconciliation mismatch")
	}
	s.Issues, s.Logos = actualIssues, actualLogos
	s.Reasons = readinessReasons(s, false)
	expected, _ := marshal(s)
	original, _ := marshal(m.Summary)
	report, err := readBytes(root, "reports/summary.json", maxMetadata)
	if err != nil || !bytes.Equal(expected, original) || !bytes.Equal(report, append(expected, '\n')) {
		return errors.New("summary JSON disagrees with verified rows")
	}
	md, err := readBytes(root, "reports/summary.md", maxMetadata)
	if err != nil || string(md) != s.markdown() {
		return errors.New("summary Markdown disagrees with JSON")
	}
	return nil
}

func validIdeas(s summary) bool {
	switch s.GFA {
	case "unavailable", "unsupported", "resource_limit":
		return len(s.Ideas) == 0
	case "aggregate_only":
		var total int64
		for kind, n := range s.Ideas {
			if !slices.Contains([]string{"total", "game", "site", "other", "unknown"}, kind) || n < 0 || n > rowLimit {
				return false
			}
			if kind != "total" {
				total += n
			}
		}
		return total == s.Ideas["total"]
	default:
		return false
	}
}

func verifyLogo(ctx context.Context, root *os.Root, files map[string]entry, obj logo, seen map[string]logo, r *record) error {
	if obj.Status != "ready" {
		if obj.Width != 0 || obj.Height != 0 || obj.Bytes != 0 || obj.SHA != "" || obj.Path != "" || obj.Key != "" || obj.Conversion != "" {
			return errors.New("unready Logo has output metadata")
		}
		switch obj.Status {
		case "missing_icon":
			if obj.SourceKey != "" {
				return errors.New("Logo key present for missing icon")
			}
		case "unsafe_key":
			if obj.SourceKey == "" || logoKeyPattern.MatchString(obj.SourceKey) {
				return errors.New("invalid unsafe Logo status")
			}
		case "logo_key_site_mismatch":
			if !logoKeyPattern.MatchString(obj.SourceKey) || logoBelongsTo(obj.SourceKey, obj.SiteID) {
				return errors.New("invalid mismatched Logo owner")
			}
		case "invalid_image", "unsupported_image_format", "source_checksum_mismatch", "conversion_failed", "missing_object", "asset_unavailable",
			"object_read_rejected", "asset_limit_exceeded", "image_input_limit_exceeded", "logo_suspected_placeholder", "logo_known_placeholder",
			"logo_svg_rejected", "logo_decode_failed", "logo_quality_rejected":
			if !logoKeyPattern.MatchString(obj.SourceKey) || !logoBelongsTo(obj.SourceKey, obj.SiteID) {
				return errors.New("invalid source Logo key")
			}
		default:
			return errors.New("invalid Logo status")
		}
		r.issue(obj.Status)
		return nil
	}
	f := files[obj.Path]
	if !logoKeyPattern.MatchString(obj.SourceKey) || !logoBelongsTo(obj.SourceKey, obj.SiteID) || !outputLogoPattern.MatchString(obj.Path) || f.Path == "" ||
		obj.SHA != f.SHA || obj.Key != "sites/logos/"+obj.SHA+".webp" || obj.Bytes != f.Bytes ||
		(obj.Conversion != "original_webp" && obj.Conversion != "lossless_webp_centered" &&
			obj.Conversion != "svg_webp_centered" && obj.Conversion != "ico_webp_centered") {
		return errors.New("Logo index disagrees with object")
	}
	if obj.Conversion == "original_webp" && strings.SplitN(filepath.Base(obj.SourceKey), ".", 2)[0] != obj.SHA[:32] {
		return errors.New("retained WebP disagrees with legacy source checksum")
	}
	if prior, ok := seen[obj.SHA]; ok {
		if prior.Width != obj.Width || prior.Height != obj.Height {
			return errors.New("Logo duplicate dimensions differ")
		}
		r.issue("logo_duplicate_content")
		return nil
	}
	raw, err := readBytes(root, obj.Path, imageOutputLimit)
	if err != nil {
		return errors.New("cannot open verified Logo object")
	}
	var store *storage.Store
	asset, err := store.ImportSiteAsset(ctx, storage.SiteLogo, bytes.NewReader(raw), true)
	if err != nil || asset.Key != obj.Key || asset.SHA256 != obj.SHA || asset.Width != obj.Width || asset.Height != obj.Height {
		return errors.New("Logo content fails existing SiteLogo verification")
	}
	seen[obj.SHA] = obj
	return nil
}
