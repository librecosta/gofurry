package legacyseed

import (
	"bufio"
	"bytes"
	"cmp"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json/v2"
	"errors"
	"fmt"
	"hash"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

const (
	formatVersion   = 1
	maxLine         = 256 << 10
	maxPackageBytes = 1 << 30
	maxMetadata     = 4 << 20
)

func marshal(v any) ([]byte, error) { return json.Marshal(v, json.Deterministic(true)) }

type tableCount struct {
	Source     int    `json:"source_rows"`
	Usable     int    `json:"exported_usable"`
	Unresolved int    `json:"exported_unresolved"`
	Excluded   int    `json:"explicitly_excluded"`
	Exclusion  string `json:"exclusion_reason"`
}

type summary struct {
	Version         int                   `json:"schema_version"`
	Tables          map[string]tableCount `json:"tables"`
	Issues          map[string]int        `json:"issues"`
	Logos           map[string]int        `json:"logos"`
	UniqueLogos     int                   `json:"unique_logos"`
	Ideas           map[string]int64      `json:"gfa_content_ideas"`
	GFA             string                `json:"gfa_status"`
	ExtractComplete bool                  `json:"extract_complete"`
	AssetComplete   bool                  `json:"asset_complete"`
	CleanSeedReady  bool                  `json:"clean_seed_ready"`
	Reasons         []string              `json:"reasons"`
}

func (s summary) markdown() string {
	var body strings.Builder
	fmt.Fprintf(&body, "# D5-A legacy seed summary\n\n- Format version %d\n- Extract complete %t\n- Asset complete %t\n- Clean seed ready %t\n- GFA aggregate %s\n- Unique Logo objects %d\n\n",
		s.Version, s.ExtractComplete, s.AssetComplete, s.CleanSeedReady, s.GFA, s.UniqueLogos)
	groups := []struct {
		title  string
		counts map[string]int
	}{{"Issues", s.Issues}, {"Logo references", s.Logos}}
	for _, group := range groups {
		fmt.Fprintf(&body, "## %s\n\n", group.title)
		keys := []string{}
		for code := range group.counts {
			keys = append(keys, code)
		}
		slices.Sort(keys)
		for _, code := range keys {
			fmt.Fprintf(&body, "- %s %d\n", code, group.counts[code])
		}
		body.WriteString("\n")
	}
	body.WriteString("## Source reconciliation\n\n| Table | Source | Usable | Unresolved | Excluded | Reason |\n| --- | ---: | ---: | ---: | ---: | --- |\n")
	for _, t := range tables {
		n, ok := s.Tables[t.name]
		if ok {
			fmt.Fprintf(&body, "| %s | %d | %d | %d | %d | %s |\n", t.name, n.Source, n.Usable, n.Unresolved, n.Excluded, n.Exclusion)
		}
	}
	if len(s.Ideas) > 0 {
		body.WriteString("\n## GFA content-idea totals only\n\n")
		for _, kind := range []string{"total", "game", "site", "other", "unknown"} {
			if count, ok := s.Ideas[kind]; ok {
				fmt.Fprintf(&body, "- %s %d\n", kind, count)
			}
		}
	}
	body.WriteString("\n## Readiness reasons\n\n")
	for _, reason := range s.Reasons {
		fmt.Fprintf(&body, "- %s\n", reason)
	}
	return body.String()
}

type entry struct {
	Path  string `json:"path"`
	SHA   string `json:"sha256"`
	Bytes int64  `json:"bytes"`
	Rows  int    `json:"rows"`
}

type manifest struct {
	Version      int       `json:"schema_version"`
	Snapshot     string    `json:"snapshot_id"`
	BackupSHA    string    `json:"backup_sha256"`
	LegacyCommit string    `json:"legacy_source_commit"`
	CreatedAt    time.Time `json:"created_at"`
	ContentSHA   string    `json:"content_sha256"`
	Files        []entry   `json:"files"`
	Summary      summary   `json:"summary"`
}

type packageWriter interface {
	row(path string, v any) error
	blob(path string, data []byte) error
}

type discardPackage struct{}

func (discardPackage) row(string, any) error     { return nil }
func (discardPackage) blob(string, []byte) error { return nil }

type packageFile struct {
	f      *os.File
	writer *bufio.Writer
	hash   hash.Hash
	entry  entry
}

type bundle struct {
	root  *os.Root
	bytes int64
	limit int64
	files map[string]*packageFile
}

func (b *bundle) file(path string) (*packageFile, error) {
	if f := b.files[path]; f != nil {
		return f, nil
	}
	f, err := b.root.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return nil, errors.New("cannot create private package file")
	}
	h := sha256.New()
	p := &packageFile{f, bufio.NewWriterSize(io.MultiWriter(f, h), 32768), h, entry{Path: path}}
	b.files[path] = p
	return p, nil
}

func (b *bundle) row(path string, v any) error {
	raw, err := marshal(v)
	if err != nil || len(raw)+1 > maxLine {
		return errors.New("legacy row is invalid or too large to encode")
	}
	f, err := b.file(path)
	if err != nil {
		return err
	}
	if err = b.write(f, append(raw, '\n')); err != nil {
		return err
	}
	f.entry.Rows++
	return nil
}

func (b *bundle) blob(path string, data []byte) error {
	if b.files[path] != nil {
		if outputLogoPattern.MatchString(path) {
			return nil
		}
		return errors.New("duplicate private package path")
	}
	f, err := b.file(path)
	if err != nil {
		return err
	}
	err = b.write(f, data)
	if finishErr := f.finish(); err == nil {
		err = finishErr
	}
	return err
}

func (b *bundle) write(f *packageFile, data []byte) error {
	n := int64(len(data))
	if n+b.bytes > b.limit {
		return errors.New("private package exceeds configured byte budget")
	}
	if _, err := f.writer.Write(data); err != nil {
		return errors.New("cannot write private package bytes")
	}
	b.bytes += n
	f.entry.Bytes += n
	return nil
}

func (f *packageFile) finish() error {
	if f.f == nil {
		return nil
	}
	flush := f.writer.Flush()
	err := f.f.Close()
	f.f = nil
	f.writer = nil
	f.entry.SHA = hex.EncodeToString(f.hash.Sum(nil))
	f.hash = nil
	if flush != nil || err != nil {
		return errors.New("cannot finish private package file")
	}
	return nil
}

func (b *bundle) entries() ([]entry, error) {
	out := make([]entry, 0, len(b.files))
	for _, f := range b.files {
		if err := f.finish(); err != nil {
			return nil, err
		}
		out = append(out, f.entry)
	}
	slices.SortFunc(out, func(a, b entry) int { return cmp.Compare(a.Path, b.Path) })
	return out, nil
}

func checksumLines(entries []entry) []byte {
	var lines bytes.Buffer
	for _, f := range entries {
		fmt.Fprintf(&lines, "%s  %s\n", f.SHA, f.Path)
	}
	return lines.Bytes()
}

func sha(data []byte) string { return fmt.Sprintf("%x", sha256.Sum256(data)) }

func (b *bundle) report(report summary) error {
	raw, err := marshal(report)
	if err != nil {
		return err
	}
	if err := b.blob("reports/summary.json", append(raw, '\n')); err != nil {
		return err
	}
	return b.blob("reports/summary.md", []byte(report.markdown()))
}

func (b *bundle) seal(c Config, s summary) (string, error) {
	entries, err := b.entries()
	if err != nil {
		return "", err
	}
	content := sha(checksumLines(entries))
	m := manifest{formatVersion, c.Snapshot, c.BackupSHA, SourceCommit, time.Now().UTC(), content, entries, s}
	raw, err := marshal(m)
	if err != nil {
		return "", err
	}
	if err := b.blob("manifest.json", append(raw, '\n')); err != nil {
		return "", err
	}
	entries, err = b.entries()
	if err != nil {
		return "", err
	}
	return content, b.blob("SHA256SUMS", checksumLines(entries))
}

func privateRoot(path string, create bool) (*os.Root, error) {
	path, err := filepath.Abs(path)
	if err != nil {
		return nil, errors.New("invalid private directory")
	}
	parent := filepath.VolumeName(path) + string(filepath.Separator)
	tail := strings.TrimPrefix(path, parent)
	for _, part := range strings.Split(tail, string(filepath.Separator)) {
		if part == "" {
			continue
		}
		parent = filepath.Join(parent, part)
		info, err := os.Lstat(parent)
		if errors.Is(err, os.ErrNotExist) && create {
			_ = os.Mkdir(parent, 0700)
			info, err = os.Lstat(parent)
		}
		if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return nil, errors.New("private directory is unavailable or linked")
		}
	}
	root, err := os.OpenRoot(path)
	if err != nil {
		return nil, errors.New("cannot open private directory")
	}
	return root, nil
}

func stage(c Config, name string, write func(*bundle) (Result, error)) (Result, error) {
	root, err := privateRoot(c.Output, true)
	if err != nil {
		return Result{}, err
	}
	defer root.Close()
	if _, err := root.Lstat(name); !errors.Is(err, os.ErrNotExist) {
		return Result{}, errors.New("snapshot output already exists or is unavailable")
	}
	temp, err := os.MkdirTemp(root.Name(), ".legacy-seed-")
	if err != nil {
		return Result{}, errors.New("cannot stage private snapshot")
	}
	tempName := filepath.Base(temp)
	defer root.RemoveAll(tempName)
	dir, err := root.OpenRoot(tempName)
	if err != nil {
		return Result{}, errors.New("cannot open private snapshot staging")
	}
	defer dir.Close()
	b := &bundle{root: dir, limit: int64(c.Limits.PackageBytes), files: map[string]*packageFile{}}
	defer func() { _, _ = b.entries() }()
	for _, d := range []string{"content", "assets/site-logos", "reports"} {
		if err := dir.MkdirAll(d, 0700); err != nil {
			return Result{}, errors.New("cannot create private snapshot directories")
		}
	}
	result, err := write(b)
	if err != nil {
		return result, err
	}
	if err := dir.Close(); err != nil {
		return result, errors.New("cannot close private snapshot staging")
	}
	if _, err := root.Lstat(name); !errors.Is(err, os.ErrNotExist) {
		return result, errors.New("snapshot output already exists or is unavailable")
	}
	if err := root.Rename(tempName, name); err != nil {
		return result, errors.New("cannot finalize private snapshot")
	}
	return result, nil
}

func readBytes(root *os.Root, path string, limit int64) ([]byte, error) {
	f, err := openPrivateFile(root, path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	raw, err := io.ReadAll(io.LimitReader(f, limit+1))
	if err != nil || int64(len(raw)) > limit {
		return nil, errors.New("invalid file size")
	}
	return raw, nil
}

func verifyFile(ctx context.Context, root *os.Root, f entry) error {
	file, err := openPrivateFile(root, f.Path)
	if err != nil {
		return err
	}
	defer file.Close()
	h := sha256.New()
	n, err := io.Copy(h, io.LimitReader(&contextReader{ctx, file}, f.Bytes+1))
	if err != nil || n != f.Bytes || hex.EncodeToString(h.Sum(nil)) != f.SHA {
		return errors.New("file length or checksum mismatch")
	}
	return nil
}

type contextReader struct {
	ctx context.Context
	r   io.Reader
}

func (r *contextReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.r.Read(p)
}
