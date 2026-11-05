package platform

import (
	"encoding/base64"
	"errors"
	"github.com/spf13/viper"
	"net"
	"strconv"
	"time"
)

type DatabaseConfig struct {
	Host                  string        `mapstructure:"host"`
	Port                  uint16        `mapstructure:"port"`
	Name                  string        `mapstructure:"name"`
	User                  string        `mapstructure:"user"`
	Password              string        `mapstructure:"password" json:"-"`
	SSLMode               string        `mapstructure:"ssl_mode"`
	ConnectTimeout        time.Duration `mapstructure:"connect_timeout"`
	MaxConnections        int32         `mapstructure:"max_connections"`
	MinConnections        int32         `mapstructure:"min_connections"`
	MaxConnectionLifetime time.Duration `mapstructure:"max_connection_lifetime"`
	MaxConnectionIdleTime time.Duration `mapstructure:"max_connection_idle_time"`
}
type ManagementConfig struct {
	Address string `mapstructure:"address"`
	Token   string `mapstructure:"token" json:"-"`
}

// Reject weak coercion and integer overflow before mapstructure narrows values.
func validateWallpaperSource(v *viper.Viper) error {
	invalid := errors.New("invalid database/management YAML field types or bounds")
	for _, section := range []string{"database", "management"} {
		if !v.IsSet(section) {
			continue
		}
		keys := []string{"host", "name", "user", "ssl_mode", "connect_timeout", "max_connection_lifetime", "max_connection_idle_time"}
		if section == "management" {
			keys = []string{"address", "token"}
		}
		for _, key := range keys {
			if _, ok := v.Get(section + "." + key).(string); !ok {
				return invalid
			}
		}
	}
	if v.IsSet("database") {
		if value := v.Get("database.password"); value != nil {
			if _, ok := value.(string); !ok {
				return invalid
			}
		}
		for key, bound := range map[string]int64{"port": 65535, "max_connections": 20, "min_connections": 20} {
			var n int64
			switch value := v.Get("database." + key).(type) {
			case int:
				n = int64(value)
			case int64:
				n = value
			case uint64:
				if value > uint64(bound) {
					return invalid
				}
				n = int64(value)
			default:
				return invalid
			}
			if n < 0 || n > bound {
				return invalid
			}
		}
	}
	return nil
}

func (c Config) ValidateWallpaperRuntime() error {
	if d := c.Database; d != nil {
		if d.Host == "" || d.Port == 0 || d.Name == "" || d.User == "" || (d.SSLMode != "disable" && d.SSLMode != "require" && d.SSLMode != "verify-full") || d.ConnectTimeout < time.Second || d.ConnectTimeout > 10*time.Second || d.MaxConnections < 1 || d.MaxConnections > 20 || d.MinConnections < 0 || d.MinConnections > d.MaxConnections || d.MaxConnectionLifetime < time.Minute || d.MaxConnectionLifetime > 24*time.Hour || d.MaxConnectionIdleTime < time.Second || d.MaxConnectionIdleTime > time.Hour {
			return errors.New("invalid bounded database configuration")
		}
	}
	if m := c.Management; m != nil {
		host, port, e := net.SplitHostPort(m.Address)
		ip := net.ParseIP(host)
		n, pe := strconv.Atoi(port)
		token, te := base64.RawURLEncoding.Strict().DecodeString(m.Token)
		if e != nil || pe != nil || n < 1 || n > 65535 || ip == nil || (host != "127.0.0.1" && host != "::1") || te != nil || len(token) != 32 || base64.RawURLEncoding.EncodeToString(token) != m.Token || c.Database == nil || c.Storage == nil || m.Address == c.Server.Address {
			return errors.New("management requires distinct literal loopback address, database, storage and a 32-byte random base64url token")
		}
	}
	return nil
}
