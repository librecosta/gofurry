package platform

import (
	"fmt"
	"net"
	"net/url"
	"strconv"
	"time"

	"github.com/spf13/viper"
)

type Config struct {
	Server struct {
		Address     string `mapstructure:"address"`
		Environment string `mapstructure:"environment"`
	} `mapstructure:"server"`
	Deployment struct {
		Region string `mapstructure:"region"`
	} `mapstructure:"deployment"`
	Logging struct {
		SlowRequest time.Duration `mapstructure:"slow_request"`
	} `mapstructure:"logging"`
	Storage *StorageConfig `mapstructure:"storage"`
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
