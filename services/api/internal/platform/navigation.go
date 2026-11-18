package platform

import (
	"encoding/base64"
	"errors"
	"net"
	"net/url"
)

type NavigationConfig struct {
	HeatEnabled bool   `mapstructure:"heat_enabled"`
	Origin      string `mapstructure:"interaction_origin"`
	HMACKey     string `mapstructure:"interaction_hmac_key" json:"-"`
}

func (n *NavigationConfig) InteractionKey() []byte {
	if n == nil {
		return nil
	}
	key, _ := base64.RawURLEncoding.Strict().DecodeString(n.HMACKey)
	return key
}

func (c Config) validateNavigation() error {
	n := c.Navigation
	if n == nil {
		return nil
	}
	if n.HeatEnabled && c.Database == nil {
		return errors.New("navigation.heat_enabled requires a regional database")
	}
	if n.Origin != "" {
		u, err := url.Parse(n.Origin)
		if err != nil || u.User != nil || u.Hostname() == "" || n.Origin != u.Scheme+"://"+u.Host ||
			u.Scheme != "https" && !(u.Scheme == "http" && (net.ParseIP(u.Hostname()).IsLoopback() || u.Hostname() == "localhost")) {
			return errors.New("navigation.interaction_origin must be an exact HTTPS origin or a loopback HTTP origin")
		}
	}
	if n.HMACKey != "" && (n.Origin == "" || len(n.InteractionKey()) != 32 || len(n.HMACKey) != 43) {
		return errors.New("navigation.interaction_hmac_key requires an origin and exactly 32 unpadded base64url bytes")
	}
	if n.HMACKey != "" && len(c.Server.TrustedProxies) == 0 && !loopbackOrigin(n.Origin) {
		return errors.New("navigation interactions require a trusted proxy for a non-loopback origin")
	}
	return nil
}

func loopbackOrigin(raw string) bool {
	u, err := url.Parse(raw)
	return err == nil && (u.Hostname() == "localhost" || net.ParseIP(u.Hostname()).IsLoopback())
}
