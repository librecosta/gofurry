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
			u.Scheme != "https" && !(u.Scheme == "http" && c.Server.Environment == "development" &&
				(net.ParseIP(u.Hostname()).IsLoopback() || u.Hostname() == "localhost")) {
			return errors.New("navigation.interaction_origin must be an exact HTTPS origin or a loopback development origin")
		}
	}
	if n.HMACKey != "" && (n.Origin == "" || len(n.InteractionKey()) != 32 || len(n.HMACKey) != 43) {
		return errors.New("navigation.interaction_hmac_key requires an origin and exactly 32 unpadded base64url bytes")
	}
	if n.HMACKey != "" && c.Server.Environment == "production" && len(c.Server.TrustedProxies) == 0 {
		return errors.New("navigation interactions require an explicitly trusted production edge")
	}
	return nil
}
