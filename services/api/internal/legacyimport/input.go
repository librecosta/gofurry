package legacyimport

import (
	"context"
	"crypto/sha256"
	"crypto/x509"
	"encoding/json/v2"
	"errors"
	"fmt"
	"io"
	"net/netip"
	"net/url"
	"os"
	"regexp"
	"strings"
	"time"
	"uuid"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/librecosta/gofurry/services/api/internal/database"
	"github.com/librecosta/gofurry/services/api/internal/platform"
	"github.com/librecosta/gofurry/services/api/internal/storage"
	"go.yaml.in/yaml/v3"
)

const deployment = "gofurry_nav_site_cn"
const schemaVersion = 202611200002

const BootstrapDeployment = deployment
const BootstrapGooseVersion = schemaVersion
const BootstrapPlanVersion = 1

var (
	digestPattern   = regexp.MustCompile(`^[0-9a-f]{64}$`)
	databasePattern = regexp.MustCompile(`^gofurry_d5b1_[a-z0-9_]{1,38}$`)
	bucketPattern   = regexp.MustCompile(`^gofurry-d5b1-[a-z0-9-]{1,38}$`)
	ErrIncomplete   = errors.New("import has unresolved or retryable units")
)

type Config struct {
	SeedDir       string `yaml:"seed_dir"`
	Snapshot      string `yaml:"snapshot_id"`
	Backup        string `yaml:"backup_sha256"`
	Content       string `yaml:"content_sha256"`
	DecisionsFile string `yaml:"decisions_file"`
	Deployment    string `yaml:"legacy_deployment"`
	Region        string `yaml:"region"`
	Environment   string `yaml:"environment"`
	Isolation     string `yaml:"isolation_token"`
	Database      struct {
		Host     string `yaml:"host"`
		Port     uint16 `yaml:"port"`
		Name     string `yaml:"name"`
		User     string `yaml:"user"`
		Password string `yaml:"password" json:"-"`
	} `yaml:"database"`
	Storage *struct {
		Endpoint      string `yaml:"endpoint"`
		PublicBaseURL string `yaml:"public_base_url"`
		Region        string `yaml:"region"`
		Bucket        string `yaml:"bucket"`
		Key           string `yaml:"access_key_id" json:"-"`
		Secret        string `yaml:"secret_access_key" json:"-"`
		CAFile        string `yaml:"ca_file"`
	} `yaml:"storage"`
	bootstrapPackage string
}

func readFile(path string, limit int64) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, errors.New("cannot open explicit input file")
	}
	defer f.Close()
	stat, err := f.Stat()
	if err != nil || !stat.Mode().IsRegular() || stat.Size() > limit {
		return nil, errors.New("invalid input file type or size")
	}
	body, err := io.ReadAll(io.LimitReader(f, limit+1))
	if err != nil || int64(len(body)) > limit {
		return nil, errors.New("cannot read bounded input file")
	}
	return body, nil
}

func LoadConfig(path string) (Config, error) {
	var c Config
	raw, err := readFile(path, 65536)
	if err != nil {
		return c, err
	}
	d := yaml.NewDecoder(strings.NewReader(string(raw)))
	d.KnownFields(true)
	var extra yaml.Node
	if d.Decode(&c) != nil || d.Decode(&extra) != io.EOF {
		return c, errors.New("expected one explicit importer YAML with known fields")
	}
	return c, c.validate()
}

func loopback(host string) bool {
	addr, err := netip.ParseAddr(host)
	return err == nil && addr.Zone() == "" && addr.IsLoopback()
}

func localHTTPS(s string) bool {
	u, err := url.Parse(s)
	return err == nil && u.Scheme == "https" && loopback(u.Hostname()) && u.User == nil && u.RawQuery == "" && !u.ForceQuery && u.Fragment == ""
}

func (c Config) validate() error {
	id, err := uuid.Parse(c.Isolation)
	if c.Deployment != deployment || c.Region != "CN" || c.Environment != "development" || err != nil || id == (uuid.UUID{}) ||
		!loopback(c.Database.Host) || !databasePattern.MatchString(c.Database.Name) || c.Database.Port == 0 ||
		c.Database.User == "" || c.Database.Password == "" || c.SeedDir == "" || c.Snapshot == "" ||
		!digestPattern.MatchString(c.Content) || !digestPattern.MatchString(c.Backup) {
		return errors.New("expected pinned seed and isolated CN development target")
	}
	if s := c.Storage; s != nil && (!localHTTPS(s.Endpoint) || !localHTTPS(s.PublicBaseURL) ||
		!bucketPattern.MatchString(s.Bucket) || s.Region == "" || s.Key == "" || s.Secret == "") {
		return errors.New("expected isolated HTTPS object store and public delivery")
	}
	return nil
}

func digest(raw []byte) string { return fmt.Sprintf("%x", sha256.Sum256(raw)) }

func encode(data any) []byte {
	raw, err := json.Marshal(data, json.Deterministic(true))
	if err != nil {
		panic(err)
	} // Only validated internal values reach this function.
	return raw
}

func (c Config) target() string {
	asset := []string{}
	if s := c.Storage; s != nil {
		asset = []string{s.Endpoint, s.PublicBaseURL, s.Bucket, s.Region}
	}
	scope := []any{c.Deployment, c.Region, c.Environment, c.Isolation, c.Database.Name, schemaVersion, asset}
	if c.bootstrapPackage != "" {
		scope = append(scope, c.bootstrapPackage, c.Database.Host, c.Database.Port)
	}
	return digest(encode(scope))
}

func (c Config) open(ctx context.Context) (*pgxpool.Pool, error) {
	db := c.Database
	pc, err := database.PoolConfig(platform.DatabaseConfig{Host: db.Host, Port: db.Port, Name: db.Name, User: db.User, Password: db.Password,
		SSLMode: "disable", ConnectTimeout: 5 * time.Second, MaxConnections: 3, MaxConnectionLifetime: time.Hour, MaxConnectionIdleTime: time.Minute})
	if err != nil {
		return nil, err
	}
	pc.ConnConfig.RuntimeParams = map[string]string{"application_name": "gofurry-d5-b1", "TimeZone": "UTC", "search_path": "public,pg_catalog",
		"statement_timeout": "15000", "lock_timeout": "3000", "idle_in_transaction_session_timeout": "15000"}
	pool, err := pgxpool.NewWithConfig(ctx, pc)
	if err != nil {
		return nil, errors.New("cannot initialize isolated PostgreSQL pool")
	}
	return pool, nil
}

func (c Config) objects() (*storage.Store, error) {
	s := c.Storage
	if s == nil {
		return nil, nil
	}
	var roots *x509.CertPool
	if s.CAFile != "" {
		body, err := readFile(s.CAFile, 65536)
		roots = x509.NewCertPool()
		if err != nil || !roots.AppendCertsFromPEM(body) {
			return nil, errors.New("invalid isolated object store TLS CA")
		}
	}
	return storage.NewWithRootCAs(platform.StorageConfig{Endpoint: s.Endpoint, PublicBaseURL: s.PublicBaseURL,
		Region: s.Region, Bucket: s.Bucket, AccessKeyID: s.Key, SecretAccessKey: s.Secret}, roots), nil
}

type rowQuerier interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}

func (c Config) guard(ctx context.Context, conn rowQuerier) error {
	var allowed bool
	err := conn.QueryRow(ctx, `SELECT coalesce(current_setting('server_version_num')::integer BETWEEN 180000 AND 189999
		AND current_database()=$1 AND current_setting('server_encoding')='UTF8'
		AND shobj_description((SELECT oid FROM pg_catalog.pg_database WHERE datname=current_database()),'pg_database')=$2
		AND (SELECT version_id FROM public.goose_db_version ORDER BY id DESC LIMIT 1)=$3
		AND (SELECT is_applied FROM public.goose_db_version ORDER BY id DESC LIMIT 1),false)`,
		c.Database.Name, "gofurry-d5-b1:"+c.Isolation, int64(schemaVersion)).Scan(&allowed)
	if err != nil || !allowed {
		return errors.New("target must be the marked PostgreSQL 18 database at the supported GoFurry Goose version")
	}
	return nil
}
