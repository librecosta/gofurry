package platform

import (
	"fmt"
	"net/url"
	"strings"
)

type QWeatherConfig struct {
	Enabled        bool   `mapstructure:"enabled"`
	APIHost        string `mapstructure:"api_host" json:"-"`
	DeveloperID    string `mapstructure:"developer_id" json:"-"`
	ProjectID      string `mapstructure:"project_id" json:"-"`
	CredentialID   string `mapstructure:"credential_id" json:"-"`
	PrivateKeyPath string `mapstructure:"private_key_path" json:"-"`
	ProxyURL       string `mapstructure:"proxy_url" json:"-"`
}

// MissingFields reports names only, never credential values. Partial optional
// configuration degrades weather; the other API services can still start.
func (q *QWeatherConfig) MissingFields() []string {
	fields := []string{}
	if q == nil {
		return []string{"api_host", "developer_id", "project_id", "credential_id", "private_key_path"}
	}
	for _, item := range []struct{ name, value string }{{"api_host", q.APIHost}, {"developer_id", q.DeveloperID}, {"project_id", q.ProjectID}, {"credential_id", q.CredentialID}, {"private_key_path", q.PrivateKeyPath}} {
		if strings.TrimSpace(item.value) == "" {
			fields = append(fields, item.name)
		}
	}
	return fields
}

func (q *QWeatherConfig) Validate() error {
	if !q.Enabled {
		return nil
	}
	if q.APIHost != "" {
		host := strings.ToLower(q.APIHost)
		if !strings.HasSuffix(host, ".qweatherapi.com") || len(host) > 253 {
			return fmt.Errorf("qweather.api_host must be the dedicated QWeather hostname")
		}
		for _, label := range strings.Split(host, ".") {
			if label == "" || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
				return fmt.Errorf("invalid qweather.api_host")
			}
			for _, c := range label {
				if !(c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '-') {
					return fmt.Errorf("invalid qweather.api_host")
				}
			}
		}
	}
	for _, value := range []string{q.DeveloperID, q.ProjectID, q.CredentialID} {
		if len(value) > 128 || strings.ContainsAny(value, "\r\n\t ") {
			return fmt.Errorf("invalid qweather credential identifier")
		}
	}
	if q.ProxyURL != "" {
		u, err := url.Parse(q.ProxyURL)
		if err != nil || u.Hostname() == "" || (u.Scheme != "http" && u.Scheme != "https" && u.Scheme != "socks5" && u.Scheme != "socks5h") || (u.Path != "" && u.Path != "/") || u.RawQuery != "" || u.Fragment != "" {
			return fmt.Errorf("qweather.proxy_url must be an explicit HTTP(S)/SOCKS proxy URL")
		}
	}
	return nil
}
