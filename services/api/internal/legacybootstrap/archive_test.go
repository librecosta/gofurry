package legacybootstrap

import (
	"archive/tar"
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/klauspost/compress/zstd"
	"github.com/librecosta/gofurry/services/api/internal/legacyimport"
	"github.com/librecosta/gofurry/services/api/internal/legacyseed"
)

func TestArchiveVerificationRejectsUnsafeInput(t *testing.T) {
	const data = "fixture"
	makeManifest := func() Manifest {
		m := Manifest{Version: 1, Purpose: purpose, Region: "CN", Deployment: legacyimport.BootstrapDeployment,
			SeedSchema: legacyseed.SchemaVersion, PlanVersion: legacyimport.BootstrapPlanVersion, GooseVersion: legacyimport.BootstrapGooseVersion,
			Seed: Seed{Snapshot: "fixture", Content: sum([]byte("content")), Backup: sum([]byte("backup"))}}
		for _, name := range []string{decisionsName, "seed/SHA256SUMS", "seed/assets/site-logos/" + strings.Repeat("a", 64) + ".webp", "seed/manifest.json"} {
			m.Files = append(m.Files, member{name, int64(len(data)), sum([]byte(data))})
		}
		return m
	}
	type fault struct {
		manifest func(*Manifest)
		header   func(*tar.Header)
		content  string
		skipLogo bool
		trailing bool
		truncate bool
		wrongSHA bool
		want     string
	}
	for name, f := range map[string]fault{
		"canonical":           {},
		"wrong_archive_sha":   {wrongSHA: true, want: "archive SHA256"},
		"broken_compression":  {truncate: true, want: "corrupt or truncated"},
		"wrong_schema":        {manifest: func(m *Manifest) { m.SeedSchema++ }, want: "incompatible"},
		"release_marker":      {manifest: func(m *Manifest) { m.Purpose = "production_ready" }, want: "incompatible"},
		"path_traversal":      {header: func(h *tar.Header) { h.Name = "seed/../../outside" }, want: "TAR structure"},
		"windows_path":        {header: func(h *tar.Header) { h.Name = "C:/outside" }, want: "TAR structure"},
		"hardlink":            {header: func(h *tar.Header) { h.Typeflag, h.Size, h.Linkname = tar.TypeLink, 0, "seed/manifest.json" }, want: "TAR structure"},
		"duplicate":           {manifest: func(m *Manifest) { m.Files[2] = m.Files[1] }, want: "TAR structure"},
		"extra_evidence":      {manifest: func(m *Manifest) { m.Files[3].Path = "seed/legacy_dump.sql" }, want: "TAR structure"},
		"missing_logo":        {skipLogo: true, want: "TAR structure"},
		"wrong_logo_bytes":    {content: "altered", want: "member size or SHA256"},
		"extra_decoded_bytes": {trailing: true, want: "archive has trailing"},
	} {
		t.Run(name, func(t *testing.T) {
			m := makeManifest()
			if f.manifest != nil {
				f.manifest(&m)
			}
			var raw bytes.Buffer
			z, err := zstd.NewWriter(&raw)
			if err != nil {
				t.Fatal(err)
			}
			tarWriter := tar.NewWriter(z)
			add := func(h *tar.Header, payload []byte) {
				t.Helper()
				if err := tarWriter.WriteHeader(h); err != nil {
					t.Fatal(err)
				}
				if h.Typeflag == tar.TypeReg {
					if _, err := tarWriter.Write(payload); err != nil {
						t.Fatal(err)
					}
				}
			}
			payload, err := canonical(m)
			if err != nil {
				t.Fatal(err)
			}
			add(header(member{Path: manifestName, Size: int64(len(payload))}), payload)
			for i, file := range m.Files {
				h := header(file)
				payload := []byte(data)
				if i == 2 {
					if f.skipLogo {
						continue
					}
					if f.header != nil {
						f.header(h)
					}
					if f.content != "" {
						payload = []byte(f.content)
					}
				}
				add(h, payload)
			}
			if err := tarWriter.Close(); err != nil {
				t.Fatal(err)
			}
			if f.trailing {
				_, _ = io.WriteString(z, "unexpected")
			}
			if err := z.Close(); err != nil {
				t.Fatal(err)
			}
			body := raw.Bytes()
			if f.truncate {
				body = body[:len(body)-1]
			}
			digest := sum(body)
			if f.wrongSHA {
				digest = sum([]byte("wrong"))
			}
			path := filepath.Join(t.TempDir(), Filename)
			if err := os.WriteFile(path, body, 0600); err != nil {
				t.Fatal(err)
			}
			input, err := os.Open(path)
			if err != nil {
				t.Fatal(err)
			}
			defer input.Close()
			_, err = unpack(context.Background(), input, digest, t.TempDir())
			if f.want == "" && err != nil || f.want != "" && (err == nil || !strings.Contains(err.Error(), f.want)) {
				t.Fatalf("got %v, want %q", err, f.want)
			}
		})
	}
}

func TestCredentialScreen(t *testing.T) {
	if err := scanJSON([]byte(`{"game":"cookie","link":"https://example.com/?lang=en"}`)); err != nil {
		t.Fatal(err)
	}
	for _, input := range []string{
		`{"private":{"access_token":"fixture-only"}}`,
		`["https://editor:fixture@example.com/"]`,
		`{"link":"https://example.com/?%74oken=fixture"}`,
		`["Authorization: Bearer fixture_only_12345"]`,
	} {
		if err := scanJSON([]byte(input)); !errors.Is(err, errSensitive) {
			t.Fatalf("credential went undetected: %v", err)
		}
	}
}
