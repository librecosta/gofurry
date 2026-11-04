package platform

import (
	"fmt"
	"net"
	"net/netip"
	"net/url"
	"path/filepath"
	"strconv"
	"time"

	"github.com/spf13/viper"
)

type Config struct {
	Server struct {
		Address        string   `mapstructure:"address"`
		Environment    string   `mapstructure:"environment"`
		TrustedProxies []string `mapstructure:"trusted_proxies"`
	} `mapstructure:"server"`
	Deployment struct {
		Region string `mapstructure:"region"`
	} `mapstructure:"deployment"`
	Logging struct {
		SlowRequest time.Duration `mapstructure:"slow_request"`
	} `mapstructure:"logging"`
	Storage           *StorageConfig           `mapstructure:"storage"`
	SearchSuggestions *SearchSuggestionsConfig `mapstructure:"search_suggestions"`
	GeoIP             *GeoIPConfig             `mapstructure:"geoip"`
	QWeather          *QWeatherConfig          `mapstructure:"qweather"`
	Cities            *CitiesConfig            `mapstructure:"cities"`
}

type CitiesConfig struct {
	IndexPath string `mapstructure:"index_path" json:"-"`
}

type GeoIPConfig struct {
	DatabasePath string `mapstructure:"database_path" json:"-"`
}

// Online autocomplete is opt-in. CN only returns exact, locally reviewed phrases;
// DuckDuckGo autocomplete does not inherit its search-results Safe Search setting.
type SearchSuggestionsConfig struct {
	Enabled     bool     `mapstructure:"enabled"`
	ProxyURL    string   `mapstructure:"proxy_url" json:"-"`
	CNAllowlist []string `mapstructure:"cn_allowlist"`
}

// S3-compatible configuration, initially used with R2. Credentials have no
// environment/instance-profile fallback and must never be serialized to clients.
type StorageConfig struct {
	Endpoint        string `mapstructure:"endpoint"`
	Region          string `mapstructure:"region"`
	Bucket          string `mapstructure:"bucket"`
	AccessKeyID     string `mapstructure:"access_key_id" json:"-"`
	SecretAccessKey string `mapstructure:"secret_access_key" json:"-"`
	PublicBaseURL   string `mapstructure:"public_base_url"`
}

// LoadConfig reads one explicit YAML file. No search paths, environment bindings,
// defaults, business flags or secondary files are merged into it.
func LoadConfig(path string) (Config, error) {
	var cfg Config
	if path == "" {
		return cfg, fmt.Errorf("--config must name a YAML file")
	}
	v := viper.New()
	v.SetConfigFile(path)
	v.SetConfigType("yaml")
	if err := v.ReadInConfig(); err != nil {
		return cfg, fmt.Errorf("read config: %w", err)
	}
	if err := v.UnmarshalExact(&cfg); err != nil {
		return cfg, fmt.Errorf("decode config: %w", err)
	}
	host, port, err := net.SplitHostPort(cfg.Server.Address)
	if err != nil || net.ParseIP(host) == nil {
		return cfg, fmt.Errorf("server.address must be an IP:port listener")
	}
	number, err := strconv.Atoi(port)
	if err != nil || number < 1 || number > 65535 {
		return cfg, fmt.Errorf("server.address port must be 1..65535")
	}
	if cfg.Server.Environment != "development" && cfg.Server.Environment != "production" {
		return cfg, fmt.Errorf("server.environment must be development or production")
	}
	if cfg.Deployment.Region != "CN" && cfg.Deployment.Region != "INTL" {
		return cfg, fmt.Errorf("deployment.region must be CN or INTL")
	}
	if cfg.Logging.SlowRequest <= 0 {
		return cfg, fmt.Errorf("logging.slow_request must be a positive duration")
	}
	for i, value := range cfg.Server.TrustedProxies {
		if ip, err := netip.ParseAddr(value); err == nil && ip.Zone() == "" && !ip.IsUnspecified() && !ip.IsMulticast() {
			cfg.Server.TrustedProxies[i] = ip.Unmap().String()
			continue
		}
		prefix, err := netip.ParsePrefix(value)
		if err != nil || prefix.Bits() == 0 || prefix.Addr().Is4In6() || prefix.Addr().IsMulticast() {
			return cfg, fmt.Errorf("server.trusted_proxies must contain explicit IPs or non-/0 CIDRs")
		}
		cfg.Server.TrustedProxies[i] = prefix.Masked().String()
	}
	if cfg.GeoIP != nil && cfg.GeoIP.DatabasePath != "" {
		// Relative paths belong to the one YAML file, not the shell's working directory.
		if !filepath.IsAbs(cfg.GeoIP.DatabasePath) {
			cfg.GeoIP.DatabasePath = filepath.Join(filepath.Dir(path), cfg.GeoIP.DatabasePath)
		}
		cfg.GeoIP.DatabasePath, err = filepath.Abs(cfg.GeoIP.DatabasePath)
		if err != nil {
			return cfg, fmt.Errorf("resolve geoip.database_path")
		}
	}
	if q := cfg.QWeather; q != nil {
		if err := q.Validate(); err != nil {
			return cfg, err
		}
		if q.PrivateKeyPath != "" {
			if !filepath.IsAbs(q.PrivateKeyPath) {
				q.PrivateKeyPath = filepath.Join(filepath.Dir(path), q.PrivateKeyPath)
			}
			q.PrivateKeyPath, err = filepath.Abs(q.PrivateKeyPath)
			if err != nil {
				return cfg, fmt.Errorf("resolve qweather.private_key_path")
			}
		}
	}
	if c := cfg.Cities; c != nil && c.IndexPath != "" {
		if !filepath.IsAbs(c.IndexPath) {
			c.IndexPath = filepath.Join(filepath.Dir(path), c.IndexPath)
		}
		c.IndexPath, err = filepath.Abs(c.IndexPath)
		if err != nil {
			return cfg, fmt.Errorf("resolve cities.index_path")
		}
	}
	if s := cfg.Storage; s != nil {
		for _, address := range []string{s.Endpoint, s.PublicBaseURL} {
			u, err := url.Parse(address)
			if err != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
				return cfg, fmt.Errorf("storage endpoint/public_base_url must be public HTTPS URLs without credentials, query or fragment")
			}
		}
		if s.Region == "" || s.Bucket == "" || s.AccessKeyID == "" || s.SecretAccessKey == "" {
			return cfg, fmt.Errorf("storage requires region, bucket and explicit credentials")
		}
	}
	if s := cfg.SearchSuggestions; s != nil {
		if s.ProxyURL != "" {
			u, err := url.Parse(s.ProxyURL)
			if err != nil || u.Hostname() == "" || (u.Scheme != "http" && u.Scheme != "https" && u.Scheme != "socks5" && u.Scheme != "socks5h") || (u.Path != "" && u.Path != "/") || u.RawQuery != "" || u.Fragment != "" {
				return cfg, fmt.Errorf("search_suggestions.proxy_url must be an explicit HTTP(S)/SOCKS proxy URL")
			}
		}
		if len(s.CNAllowlist) > 256 {
			return cfg, fmt.Errorf("search_suggestions.cn_allowlist exceeds 256 phrases")
		}
		for _, phrase := range s.CNAllowlist {
			if len(phrase) == 0 || len([]rune(phrase)) > 128 {
				return cfg, fmt.Errorf("search_suggestions.cn_allowlist phrases must contain 1..128 characters")
			}
		}
		if s.Enabled && cfg.Deployment.Region == "CN" && len(s.CNAllowlist) == 0 {
			return cfg, fmt.Errorf("CN online suggestions require a reviewed cn_allowlist; otherwise set enabled: false")
		}
	}
	return cfg, nil
}

func (c Config) ProbeURL() string {
	host, port, _ := net.SplitHostPort(c.Server.Address)
	if net.ParseIP(host).IsUnspecified() {
		if net.ParseIP(host).To4() != nil {
			host = "127.0.0.1"
		} else {
			host = "::1"
		}
	}
	return "http://" + net.JoinHostPort(host, port) + "/readyz"
}
