package platform

import (
	"encoding/base64"
	"errors"
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
