package platform

import (
	"errors"
	"net/netip"
	"net/url"
	"time"
)

type DiscoveryConfig struct {
	Enabled          bool               `mapstructure:"enabled"`
	Namespace        string             `mapstructure:"namespace"`
	ReaderToken      string             `mapstructure:"reader_token" json:"-"`
	WriterToken      string             `mapstructure:"writer_token" json:"-"`
	PollInterval     time.Duration      `mapstructure:"poll_interval"`
	ScanInterval     time.Duration      `mapstructure:"scan_interval"`
	PageSize         int32              `mapstructure:"page_size"`
	JobsPerDay       int32              `mapstructure:"jobs_per_day"`
	CandidatesPerDay int32              `mapstructure:"candidates_per_day"`
	ExplorePerDay    int32              `mapstructure:"explore_per_day"`
	Seeds            []uint32           `mapstructure:"seeds"`
	AI               *DiscoveryAIConfig `mapstructure:"ai" json:"-"`
}

type DiscoveryAIConfig struct {
	URL         string `mapstructure:"url"`
	Token       string `mapstructure:"token" json:"-"`
	Model       string `mapstructure:"model"`
	CallsPerDay int32  `mapstructure:"calls_per_day"`
}

func (c *Config) validateDiscovery(h *HarvestConfig) error {
	d := h.Discovery
	if d == nil {
		return nil
	}
	if !h.Consume || h.IntakeEgressRegion == "" || !harvestNamespace.MatchString(d.Namespace) || d.Namespace == h.Namespace ||
		!harvestToken.MatchString(d.ReaderToken) || !harvestToken.MatchString(d.WriterToken) ||
		d.WriterToken == d.ReaderToken || d.ReaderToken == h.ReaderToken || d.ReaderToken == h.WriterToken ||
		d.WriterToken == h.ReaderToken || d.WriterToken == h.WriterToken || d.Enabled && !h.IntakeEnabled {
		return errors.New("harvest discovery requires Intake and separate scoped catalog credentials")
	}
	if d.PollInterval == 0 {
		d.PollInterval = 10 * time.Second
	}
	if d.ScanInterval == 0 {
		d.ScanInterval = time.Hour
	}
	if d.PageSize == 0 {
		d.PageSize = 64
	}
	if d.JobsPerDay == 0 {
		d.JobsPerDay = 256
	}
	if d.CandidatesPerDay == 0 {
		d.CandidatesPerDay = 24
	}
	if d.PollInterval < time.Second || d.PollInterval > time.Minute || d.ScanInterval < time.Minute || d.ScanInterval > 24*time.Hour ||
		d.PageSize < 1 || d.PageSize > 64 || d.JobsPerDay < 1 || d.JobsPerDay > 1440 || d.CandidatesPerDay < 1 || d.CandidatesPerDay > 128 ||
		d.ExplorePerDay < 0 || d.ExplorePerDay > d.CandidatesPerDay/4 || len(d.Seeds) > 64 {
		return errors.New("harvest discovery settings are outside supported bounds")
	}
	for _, key := range d.Seeds {
		if key == 0 {
			return errors.New("invalid harvest discovery seed")
		}
	}
	if a := d.AI; a != nil {
		u, err := url.Parse(a.URL)
		if err != nil || u.Hostname() == "" || u.User != nil || u.Path != "" && u.Path != "/" || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" {
			return errors.New("discovery.ai.url requires a dedicated AI Gateway origin")
		}
		ip, _ := netip.ParseAddr(u.Hostname())
		if u.Scheme != "https" && !(u.Scheme == "http" && c.Server.Environment == "development" && ip.IsLoopback()) {
			return errors.New("discovery.ai.url requires HTTPS or development loopback HTTP")
		}
		if !harvestToken.MatchString(a.Token) || !harvestNamespace.MatchString(a.Model) || a.CallsPerDay < 1 || a.CallsPerDay > 64 {
			return errors.New("discovery.ai requires an explicit Gateway token, logical model and daily budget")
		}
	}
	return nil
}
