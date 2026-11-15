package legacyseed

import (
	"bytes"
	"errors"
	"io"
	"net/netip"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"go.yaml.in/yaml/v3"
)

const SourceCommit = "27d4c1dbd09c5c50c9aedecd5f816d50ae92dd3f"

var snapshotPattern = regexp.MustCompile("^[a-z0-9][a-z0-9_-]{0,63}$")
var digestPattern = regexp.MustCompile("^[a-f0-9]{64}$")
var databasePattern = regexp.MustCompile("^legacy_d5_[a-z0-9_]{1,53}$")

type Database struct {
	Host     string `yaml:"host"`
	Port     uint16 `yaml:"port"`
	Name     string `yaml:"name"`
	User     string `yaml:"user"`
	Password string `yaml:"password"`
}

type Config struct {
	Snapshot    string   `yaml:"snapshot_id"`
	BackupSHA   string   `yaml:"backup_sha256"`
	TargetDB    string   `yaml:"gofurry_database"`
	Output      string   `yaml:"output_dir"`
	Logos       string   `yaml:"offline_logo_dir"`
	SVGRenderer string   `yaml:"logo_svg_renderer,omitempty"`
	GFN         Database `yaml:"gfn"`
	GFG         Database `yaml:"gfg"`
	GFA         Database `yaml:"gfa"`
	Limits      Limits   `yaml:"limits,omitempty"`
}

type Limits struct {
	SourceRows   int `yaml:"max_source_rows,omitempty"`
	PackageBytes int `yaml:"max_package_bytes,omitempty"`
	LogoObjects  int `yaml:"max_logo_objects,omitempty"`
	RunSeconds   int `yaml:"max_run_seconds,omitempty"`
}

func (l *Limits) bound() bool {
	for _, v := range []struct {
		setting *int
		max     int
	}{
		{&l.SourceRows, rowLimit}, {&l.PackageBytes, maxPackageBytes}, {&l.LogoObjects, logoReadLimit}, {&l.RunSeconds, 15 * 60},
	} {
		if *v.setting < 0 || *v.setting > v.max {
			return false
		}
		if *v.setting == 0 {
			*v.setting = v.max
		}
	}
	return true
}

func nested(path, parent string) bool {
	rel, err := filepath.Rel(parent, path)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func LoadConfig(path, output string) (Config, error) {
	var c Config
	if path == "" {
		return c, errors.New("explicit --config is required")
	}
	f, err := os.Open(path)
	if err != nil {
		return c, errors.New("cannot open private YAML")
	}
	defer f.Close()
	raw, err := io.ReadAll(io.LimitReader(f, (32<<10)+1))
	if err != nil || len(raw) > 32<<10 {
		return c, errors.New("cannot read private YAML within 32 KiB")
	}
	d := yaml.NewDecoder(bytes.NewReader(raw))
	d.KnownFields(true)
	var extra any
	if d.Decode(&c) != nil || d.Decode(&extra) != io.EOF {
		return Config{}, errors.New("invalid private YAML or unknown fields")
	}
	if !snapshotPattern.MatchString(c.Snapshot) || !digestPattern.MatchString(c.BackupSHA) || c.TargetDB == "" {
		return Config{}, errors.New("invalid snapshot_id, backup_sha256 or gofurry_database")
	}
	seen := map[string]bool{c.TargetDB: true}
	for _, p := range []Database{c.GFN, c.GFG, c.GFA} {
		ip, err := netip.ParseAddr(p.Host)
		if err != nil || !ip.IsLoopback() || ip.Zone() != "" || p.Port == 0 || !databasePattern.MatchString(p.Name) ||
			seen[p.Name] || !databasePattern.MatchString(p.User) || p.Password == "" {
			return Config{}, errors.New("sources must have distinct legacy_d5_ database names and explicit loopback endpoint and dedicated credentials")
		}
		seen[p.Name] = true
	}
	if output != "" {
		if !filepath.IsAbs(output) {
			return Config{}, errors.New("--output must be an absolute directory")
		}
		c.Output = output
	}
	if c.Output == "" {
		c.Output = ".local/legacy-seed"
		if _, err := os.Stat("AGENTS.md"); err != nil {
			return Config{}, errors.New("run from the repository root or set an absolute output_dir")
		}
	} else if !filepath.IsAbs(c.Output) {
		return Config{}, errors.New("output_dir must be absolute")
	}
	if c.Logos != "" && !filepath.IsAbs(c.Logos) {
		return Config{}, errors.New("offline_logo_dir must be absolute")
	}
	if c.SVGRenderer != "" && (!filepath.IsAbs(c.SVGRenderer) || c.Logos == "") {
		return Config{}, errors.New("logo_svg_renderer requires an absolute executable path and offline_logo_dir")
	}
	absoluteOutput, err := filepath.Abs(c.Output)
	if err != nil {
		return Config{}, errors.New("invalid output directory")
	}
	if c.Logos != "" && (nested(absoluteOutput, c.Logos) || nested(c.Logos, absoluteOutput)) {
		return Config{}, errors.New("output_dir and offline_logo_dir must be separate directories")
	}
	if !c.Limits.bound() {
		return Config{}, errors.New("limits exceed D5-A hard ceilings or are negative")
	}
	c.Output = absoluteOutput
	return c, nil
}
