package legacyseed

import (
	"bufio"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"os"

	"encoding/json/jsontext"
	"encoding/json/v2"
)

// Bundle exposes a sealed D5-A directory to offline consumers. It has no
// database configuration. Each subsequent read checks the bytes consumed
// against the already verified manifest.
type Bundle struct {
	root  *os.Root
	files map[string]entry
}

func OpenBundle(ctx context.Context, path, snapshot, backup, content string) (*Bundle, error) {
	if !snapshotPattern.MatchString(snapshot) || !digestPattern.MatchString(backup) || !digestPattern.MatchString(content) {
		return nil, errors.New("invalid independently pinned seed identity")
	}
	root, err := privateRoot(path, false)
	if err != nil {
		return nil, errors.New("cannot open private seed directory")
	}
	m, err := verifyBundle(ctx, root, Config{Snapshot: snapshot, BackupSHA: backup})
	if err != nil || m.ContentSHA != content {
		root.Close()
		return nil, errors.New("D5-A v1 seed verification failed")
	}
	b := &Bundle{root: root, files: make(map[string]entry, len(m.Files))}
	for _, f := range m.Files {
		b.files[f.Path] = f
	}
	return b, nil
}

func (b *Bundle) Close() error { return b.root.Close() }

func (b *Bundle) Rows(ctx context.Context, path string, each func(jsontext.Value) error) error {
	f, ok := b.files[path]
	if !ok || !isNDJSON(path) {
		return errors.New("requested file is outside the verified NDJSON inventory")
	}
	file, err := openPrivateFile(b.root, path)
	if err != nil {
		return errors.New("could not open verified seed rows")
	}
	defer file.Close()
	h := sha256.New()
	limit := &io.LimitedReader{R: &contextReader{ctx, file}, N: f.Bytes + 1}
	reader := &lineReader{r: bufio.NewReaderSize(io.TeeReader(limit, h), maxLine)}
	for {
		var raw jsontext.Value
		err := reader.next(&raw)
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		if err := each(raw); err != nil {
			return err
		}
	}
	if f.Rows != reader.count || limit.N != 1 || fmt.Sprintf("%x", h.Sum(nil)) != f.SHA {
		return errors.New("verified seed changed during consumption")
	}
	return nil
}

func isNDJSON(path string) bool {
	if path == logoFile || path == mappingsFile || path == exclusionsFile || path == issuesFile {
		return true
	}
	for _, t := range tables {
		if t.path() == path {
			return true
		}
	}
	return false
}

func (b *Bundle) Content(ctx context.Context, each func(Record) error) error {
	seen := map[string]bool{}
	for _, t := range tables {
		path := t.path()
		if seen[path] {
			continue
		}
		seen[path] = true
		if err := b.Rows(ctx, path, func(raw jsontext.Value) error {
			var row Record
			if json.Unmarshal(raw, &row, json.RejectUnknownMembers(true)) != nil {
				return errors.New("invalid verified seed row")
			}
			return each(row)
		}); err != nil {
			return err
		}
	}
	return nil
}

type Record = wireRecord
type Source = source
type Logo = logo
type MappingCandidate = mappingRow

func (b *Bundle) LogoBytes(sha256 string) ([]byte, error) {
	if !digestPattern.MatchString(sha256) {
		return nil, errors.New("invalid verified logo identity")
	}
	path := "assets/site-logos/" + sha256 + ".webp"
	f, ok := b.files[path]
	if !ok {
		return nil, errors.New("logo absent from verified manifest")
	}
	raw, err := readBytes(b.root, path, imageOutputLimit)
	if err != nil || int64(len(raw)) != f.Bytes || sha(raw) != sha256 {
		return nil, errors.New("verified logo changed during consumption")
	}
	return raw, nil
}
