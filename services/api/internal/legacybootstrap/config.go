package legacybootstrap

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"regexp"

	"github.com/librecosta/gofurry/services/api/internal/legacyimport"
	"go.yaml.in/yaml/v3"
)

const Filename = "gofurry-bootstrap-v2.tar.zst"

var digestPattern = regexp.MustCompile(`^[0-9a-f]{64}$`)

type Seed struct {
	Snapshot string `yaml:"snapshot_id" json:"snapshot_id"`
	Backup   string `yaml:"backup_sha256" json:"backup_sha256"`
	Content  string `yaml:"content_sha256" json:"content_sha256"`
}

type BuildConfig struct {
	Archive       string `yaml:"archive"`
	SeedDir       string `yaml:"seed_dir"`
	DecisionsFile string `yaml:"decisions_file"`
	Seed          Seed   `yaml:",inline"`
}

type InstallConfig struct {
	Archive string               `yaml:"archive"`
	SHA     string               `yaml:"package_sha256"`
	WorkDir string               `yaml:"work_dir"`
	Target  *legacyimport.Config `yaml:"target,omitempty"`
}

func LoadBuildConfig(path string) (BuildConfig, error) {
	var c BuildConfig
	err := readYAML(path, &c)
	return c, err
}

func LoadInstallConfig(path string) (InstallConfig, error) {
	var c InstallConfig
	err := readYAML(path, &c)
	return c, err
}

func readYAML(path string, value any) error {
	if !filepath.IsAbs(path) {
		return errors.New("explicit absolute --config is required")
	}
	raw, err := privateFile(path, 64<<10)
	if err != nil {
		return err
	}
	d := yaml.NewDecoder(bytes.NewReader(raw))
	d.KnownFields(true)
	var extra yaml.Node
	if d.Decode(value) != nil || d.Decode(&extra) != io.EOF {
		return errors.New("expected one bootstrap YAML with known fields")
	}
	return nil
}

func privateFile(path string, limit int64) ([]byte, error) {
	f, err := openFile(path, limit)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	raw, err := io.ReadAll(io.LimitReader(f, limit+1))
	if err != nil || int64(len(raw)) > limit {
		return nil, errors.New("bootstrap input exceeds its size limit")
	}
	return raw, nil
}

func openFile(path string, limit int64) (*os.File, error) {
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Size() > limit {
		return nil, errors.New("bootstrap input must be an existing regular file within its size limit")
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, errors.New("cannot open private bootstrap input")
	}
	current, err := f.Stat()
	if err != nil || !os.SameFile(info, current) {
		f.Close()
		return nil, errors.New("bootstrap input changed while opening")
	}
	return f, nil
}
