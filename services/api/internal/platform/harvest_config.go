package platform

import (
	"errors"
	"net/netip"
	"net/url"
	"regexp"
	"time"
	"uuid"
)

type HarvestConfig struct {
	URL                string        `mapstructure:"url" json:"-"`
	Namespace          string        `mapstructure:"namespace"`
	WriterToken        string        `mapstructure:"writer_token" json:"-"`
	ReaderToken        string        `mapstructure:"reader_token" json:"-"`
	ConsumerID         string        `mapstructure:"consumer_id"`
	Consume            bool          `mapstructure:"consume"`
	PollInterval       time.Duration `mapstructure:"poll_interval" json:"-"`
	PageSize           int           `mapstructure:"page_size"`
	IntakeEnabled      bool          `mapstructure:"intake_enabled"`
	IntakeEgressRegion string        `mapstructure:"intake_egress_region"`
}

var harvestNamespace = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,62}$`)
var harvestToken = regexp.MustCompile(`^[A-Za-z0-9_-]{32,128}$`)

func (c *Config) validateHarvest() error {
	h := c.Harvest
	if h == nil {
		return nil
	}
	if c.Deployment.Region != "CN" || c.Database == nil {
		return errors.New("harvest requires the CN deployment and a regional database")
	}
	u, err := url.Parse(h.URL)
	if err != nil || u.Hostname() == "" || u.User != nil || (u.Path != "" && u.Path != "/") || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" {
		return errors.New("harvest.url requires a private origin without credentials")
	}
	ip, _ := netip.ParseAddr(u.Hostname())
	if u.Scheme != "https" && !(u.Scheme == "http" && c.Server.Environment == "development" && ip.IsLoopback()) {
		return errors.New("harvest.url requires HTTPS or development loopback HTTP")
	}
	if !harvestNamespace.MatchString(h.Namespace) || h.WriterToken != "" && !harvestToken.MatchString(h.WriterToken) {
		return errors.New("invalid harvest namespace or writer credential")
	}
	if h.Consume {
		id, err := uuid.Parse(h.ConsumerID)
		if err != nil || id[6]>>4 != 7 || id[8]>>6 != 2 || !harvestToken.MatchString(h.ReaderToken) || h.ReaderToken == h.WriterToken {
			return errors.New("harvest consumer requires a UUIDv7 and a distinct reader credential")
		}
	} else if h.ConsumerID != "" || h.ReaderToken != "" {
		return errors.New("harvest reader settings require consume")
	}
	if !h.Consume && h.WriterToken == "" {
		return errors.New("harvest requires an enabled consumer or a writer credential")
	}
	if h.IntakeEnabled && h.IntakeEgressRegion == "" ||
		h.IntakeEgressRegion != "" && (!harvestNamespace.MatchString(h.IntakeEgressRegion) || len(h.IntakeEgressRegion) > 35 ||
			!h.Consume || h.WriterToken == "") {
		return errors.New("harvest intake requires a writer, consumer and explicit intake_egress_region")
	}
	if h.PollInterval == 0 {
		h.PollInterval = 5 * time.Second
	}
	if h.PageSize == 0 {
		h.PageSize = 32
	}
	if h.PollInterval < time.Second || h.PollInterval > 5*time.Minute || h.PageSize < 1 || h.PageSize > 100 {
		return errors.New("harvest poll_interval or page_size is outside its supported bounds")
	}
	return nil
}
