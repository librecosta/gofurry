package legacybootstrap

import (
	"archive/tar"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json/v2"
	"errors"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/klauspost/compress/zstd"
	"github.com/librecosta/gofurry/services/api/internal/legacyimport"
	"github.com/librecosta/gofurry/services/api/internal/legacyseed"
)

const (
	acceptancePurpose     = "isolated_acceptance"
	initializationPurpose = "catalog_initialization"
	manifestName          = "bootstrap.json"
	decisionsName         = "decisions.json"
	archiveLimit          = 1088 << 20
	manifestLimit         = 2 << 20
	decisionLimit         = 32 << 20
	fileLimit             = 5035
)

var (
	seedPaths      = regexp.MustCompile(`^seed/(?:SHA256SUMS|manifest\.json|(?:content|reports)/[a-z0-9_-]+\.(?:ndjson|json|md)|assets/site-logo-index\.ndjson|assets/site-logos/[a-f0-9]{64}\.webp)$`)
	epoch          = time.Unix(0, 0).UTC()
	errTar         = errors.New("invalid bootstrap TAR structure, order or metadata")
	errDigest      = errors.New("bootstrap member size or SHA256 mismatch")
	errCompression = errors.New("bootstrap archive compression is corrupt or truncated")
)

type member struct {
	Path string `json:"path"`
	Size int64  `json:"bytes"`
	SHA  string `json:"sha256"`
}

type Manifest struct {
	Version      int                             `json:"package_version"`
	Purpose      string                          `json:"purpose"`
	Region       string                          `json:"region"`
	SeedSchema   int                             `json:"seed_schema_version"`
	PlanVersion  int                             `json:"importer_plan_version"`
	GooseVersion int                             `json:"goose_version"`
	Deployment   string                          `json:"legacy_deployment"`
	Seed         Seed                            `json:"seed"`
	Inventory    legacyimport.BootstrapInventory `json:"inventory"`
	Files        []member                        `json:"files"`
}

type Report struct {
	Status               string               `json:"status"`
	BootstrapReadiness   string               `json:"bootstrap_readiness,omitempty"`
	PublicationReadiness string               `json:"publication_readiness,omitempty"`
	PackageSHA           string               `json:"package_sha256,omitempty"`
	Manifest             *Manifest            `json:"manifest,omitempty"`
	Import               *legacyimport.Report `json:"import,omitempty"`
}

func Build(ctx context.Context, c BuildConfig) (Report, error) {
	report := Report{Status: "failed"}
	if !filepath.IsAbs(c.Archive) || filepath.Base(c.Archive) != Filename ||
		!filepath.IsAbs(c.SeedDir) || !filepath.IsAbs(c.DecisionsFile) {
		return report, errors.New("build requires absolute private input paths and the v2 archive filename")
	}
	decisions, err := privateFile(c.DecisionsFile, decisionLimit)
	if err != nil {
		return report, err
	}
	b, inventory, err := openSeed(ctx, c.SeedDir, c.Seed, decisions, legacyimport.BootstrapInitialization)
	if err != nil {
		return report, err
	}
	defer b.Close()
	m := Manifest{Version: legacyimport.BootstrapInitialization, Purpose: initializationPurpose, Region: "CN", SeedSchema: legacyseed.SchemaVersion,
		PlanVersion: legacyimport.BootstrapPlanVersion, GooseVersion: legacyimport.BootstrapGooseVersion,
		Deployment: legacyimport.BootstrapDeployment, Seed: c.Seed, Inventory: inventory,
		Files: []member{{decisionsName, int64(len(decisions)), sum(decisions)}}}
	if err = b.Export(ctx, func(path string, size int64, digest string, _ io.Reader) error {
		m.Files = append(m.Files, member{"seed/" + path, size, digest})
		return nil
	}); err != nil {
		return report, err
	}
	if err = validateManifest(m); err != nil {
		return report, err
	}
	report.PackageSHA, err = writeArchive(ctx, c.Archive, m, b, decisions)
	if err != nil {
		return report, err
	}
	report.Status, report.Manifest = "built_for_initialization", &m
	report.PublicationReadiness = "INCOMPLETE"
	return report, nil
}

func openSeed(ctx context.Context, path string, identity Seed, decisions []byte, version int) (*legacyseed.Bundle, legacyimport.BootstrapInventory, error) {
	b, err := legacyseed.OpenBundle(ctx, path, identity.Snapshot, identity.Backup, identity.Content)
	if err != nil {
		return nil, legacyimport.BootstrapInventory{}, err
	}
	inventory, err := legacyimport.InspectBootstrap(ctx, b, identity.Content, decisions, version)
	if err == nil {
		err = screen(ctx, b, decisions)
	}
	if err != nil {
		b.Close()
		return nil, inventory, err
	}
	return b, inventory, nil
}

func validateManifest(m Manifest) error {
	validPurpose := m.Version == legacyimport.BootstrapAcceptance && m.Purpose == acceptancePurpose ||
		m.Version == legacyimport.BootstrapInitialization && m.Purpose == initializationPurpose
	if !validPurpose || m.Region != "CN" || m.Deployment != legacyimport.BootstrapDeployment ||
		m.SeedSchema != legacyseed.SchemaVersion || m.PlanVersion != legacyimport.BootstrapPlanVersion || m.GooseVersion != legacyimport.BootstrapGooseVersion {
		return errors.New("incompatible bootstrap package, Seed schema, importer or Goose version")
	}
	if len(m.Files) < 4 || len(m.Files) > fileLimit || m.Files[0].Path != decisionsName ||
		!digestPattern.MatchString(m.Seed.Backup) || !digestPattern.MatchString(m.Seed.Content) {
		return errTar
	}
	var size int64
	last := manifestName
	for _, f := range m.Files {
		if f.Path <= last || f.Size < 0 || f.Size > archiveLimit ||
			!digestPattern.MatchString(f.SHA) || f.Path != decisionsName && !seedPaths.MatchString(f.Path) ||
			f.Path == decisionsName && f.Size > decisionLimit {
			return errTar
		}
		last = f.Path
		size += f.Size
		if size > archiveLimit {
			return errTar
		}
	}
	return nil
}

func writeArchive(ctx context.Context, path string, m Manifest, b *legacyseed.Bundle, decisions []byte) (string, error) {
	raw, err := canonical(m)
	if err != nil || len(raw) > manifestLimit {
		return "", errors.New("cannot create bounded bootstrap manifest")
	}
	output, err := os.CreateTemp(filepath.Dir(path), ".bootstrap-*.part")
	if err != nil {
		return "", errors.New("cannot create private bootstrap staging file")
	}
	defer os.Remove(output.Name())
	defer output.Close()
	hash := sha256.New()
	z, err := zstd.NewWriter(io.MultiWriter(output, hash), zstd.WithEncoderConcurrency(1), zstd.WithEncoderCRC(true), zstd.WithWindowSize(1<<20))
	if err != nil {
		return "", err
	}
	defer z.Close()
	t := tar.NewWriter(z)
	defer t.Close()
	add := func(f member, r io.Reader) error {
		if err := t.WriteHeader(header(f)); err != nil {
			return err
		}
		return copyMember(t, r, f)
	}
	if err = add(member{manifestName, int64(len(raw)), sum(raw)}, bytes.NewReader(raw)); err == nil {
		err = add(m.Files[0], bytes.NewReader(decisions))
	}
	if err == nil {
		err = b.Export(ctx, func(path string, size int64, digest string, data io.Reader) error {
			return add(member{"seed/" + path, size, digest}, data)
		})
	}
	if err != nil {
		return "", errors.New("cannot serialize verified bootstrap members")
	}
	if t.Close() != nil || z.Close() != nil || output.Sync() != nil || output.Close() != nil {
		return "", errors.New("cannot seal bootstrap archive")
	}
	if err = os.Link(output.Name(), path); err != nil {
		return "", errors.New("cannot publish archive without overwriting an existing file")
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func header(f member) *tar.Header {
	return &tar.Header{Typeflag: tar.TypeReg, Name: f.Path, Mode: 0600, Size: f.Size, ModTime: epoch, Format: tar.FormatUSTAR}
}

func validHeader(h *tar.Header, f member) bool {
	return h.Typeflag == tar.TypeReg && h.Format == tar.FormatUSTAR && h.Name == f.Path &&
		h.Size == f.Size && h.Mode == 0600 && h.ModTime.Equal(epoch) && h.Uid == 0 && h.Gid == 0 &&
		h.Uname == "" && h.Gname == "" && h.Linkname == "" && h.Devmajor == 0 && h.Devminor == 0 &&
		h.AccessTime.IsZero() && h.ChangeTime.IsZero() && len(h.PAXRecords) == 0
}

func copyMember(out io.Writer, input io.Reader, f member) error {
	h := sha256.New()
	n, err := io.CopyN(io.MultiWriter(out, h), input, f.Size)
	if err != nil || n != f.Size || hex.EncodeToString(h.Sum(nil)) != f.SHA {
		return errDigest
	}
	return nil
}

func canonical(v any) ([]byte, error) {
	raw, err := json.Marshal(v, json.Deterministic(true))
	return append(raw, '\n'), err
}

func sum(raw []byte) string {
	d := sha256.Sum256(raw)
	return hex.EncodeToString(d[:])
}

type contextReader struct {
	ctx context.Context
	r   io.Reader
}

func (r contextReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.r.Read(p)
}

func knownSeedDirectory(name string) bool {
	switch name {
	case "seed", "seed/content", "seed/reports", "seed/assets", "seed/assets/site-logos":
		return true
	}
	return false
}

func installTarget(c *legacyimport.Config, m Manifest, dir string) (legacyimport.Config, error) {
	if c == nil {
		return legacyimport.Config{}, errors.New("bootstrap target is required for plan, apply and verify-target")
	}
	if c.SeedDir != "" || c.Snapshot != "" || c.Content != "" || c.Backup != "" || c.DecisionsFile != "" ||
		(c.Deployment != "" && c.Deployment != m.Deployment) {
		return legacyimport.Config{}, errors.New("target YAML must provide only isolated environment settings; Seed identity comes from the archive")
	}
	target := *c
	target.SeedDir, target.DecisionsFile = filepath.Join(dir, "seed"), filepath.Join(dir, decisionsName)
	target.Deployment, target.Snapshot, target.Content, target.Backup = m.Deployment, m.Seed.Snapshot, m.Seed.Content, m.Seed.Backup
	if target.Storage != nil && target.Storage.CAFile != "" && !filepath.IsAbs(target.Storage.CAFile) {
		return legacyimport.Config{}, errors.New("target storage ca_file must be absolute")
	}
	return target, nil
}

func unpack(ctx context.Context, f *os.File, digest string, dir string) (Manifest, error) {
	var m Manifest
	hash := sha256.New()
	if n, err := io.Copy(hash, io.LimitReader(contextReader{ctx, f}, archiveLimit+1)); err != nil || n > archiveLimit ||
		hex.EncodeToString(hash.Sum(nil)) != digest {
		return m, errors.New("bootstrap archive SHA256 mismatch or unreadable input")
	}
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return m, errTar
	}
	hash.Reset()
	source := io.LimitReader(contextReader{ctx, f}, archiveLimit+1)
	z, err := zstd.NewReader(io.TeeReader(source, hash), zstd.WithDecoderConcurrency(1),
		zstd.WithDecoderMaxMemory(64<<20), zstd.WithDecoderMaxWindow(8<<20))
	if err != nil {
		return m, errTar
	}
	defer z.Close()
	tr := tar.NewReader(z)
	h, err := tr.Next()
	if err != nil {
		return m, tarReadError(err)
	}
	if h.Size < 1 || h.Size > manifestLimit || !validHeader(h, member{Path: manifestName, Size: h.Size}) {
		return m, errTar
	}
	raw, err := io.ReadAll(tr)
	if err != nil || json.Unmarshal(raw, &m, json.RejectUnknownMembers(true)) != nil {
		return m, errors.New("invalid bootstrap manifest JSON")
	}
	if expected, _ := canonical(m); !bytes.Equal(raw, expected) {
		return m, errors.New("bootstrap manifest is not canonical")
	}
	if err = validateManifest(m); err != nil {
		return m, err
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		return m, errors.New("cannot open private bootstrap work directory")
	}
	defer root.Close()
	for _, e := range m.Files {
		h, err = tr.Next()
		if err != nil {
			return m, tarReadError(err)
		}
		if !validHeader(h, e) {
			return m, errTar
		}
		parent, _, _ := strings.Cut(e.Path, "/")
		if parent == "seed" {
			parent = filepath.ToSlash(filepath.Dir(e.Path))
			if !knownSeedDirectory(parent) || root.MkdirAll(parent, 0700) != nil {
				return m, errTar
			}
		}
		output, err := root.OpenFile(e.Path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if err != nil {
			return m, errors.New("cannot stage verified bootstrap member")
		}
		copyErr := copyMember(output, tr, e)
		if closeErr := output.Close(); copyErr != nil || closeErr != nil {
			return m, errDigest
		}
	}
	if _, err = tr.Next(); err != io.EOF {
		return m, errTar
	}
	var extra [1]byte
	if n, tail := io.ReadFull(z, extra[:]); n != 0 || tail != io.EOF || hex.EncodeToString(hash.Sum(nil)) != digest {
		return m, errors.New("bootstrap archive has trailing bytes, corrupt compression or changed during verification")
	}
	// D5-A requires empty directories as well when the Seed has no Logo bytes.
	if root.MkdirAll("seed/assets/site-logos", 0700) != nil {
		return m, errTar
	}
	return m, nil
}

func tarReadError(err error) error {
	if errors.Is(err, io.EOF) || errors.Is(err, tar.ErrHeader) || errors.Is(err, tar.ErrInsecurePath) {
		return errTar
	}
	return errCompression
}
