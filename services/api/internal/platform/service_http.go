package platform

import (
	"context"
	"errors"
	"net"
	"net/netip"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"time"
)

var serviceName = regexp.MustCompile("^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?$")

func validServiceOrigin(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil || u.Hostname() == "" || u.User != nil || u.Path != "" && u.Path != "/" ||
		u.RawQuery != "" || u.ForceQuery || strings.ContainsAny(raw, "#\r\n\t ") || strings.HasSuffix(u.Host, ":") {
		return false
	}
	if u.Port() != "" {
		port, err := strconv.Atoi(u.Port())
		if err != nil || port < 1 || port > 65535 {
			return false
		}
	}
	return u.Scheme == "https" || u.Scheme == "http" &&
		(privateServiceIP(u.Hostname()) || serviceName.MatchString(u.Hostname()))
}

func privateServiceIP(host string) bool {
	ip, err := netip.ParseAddr(host)
	ip = ip.Unmap()
	return err == nil && ip.Zone() == "" && (ip.IsPrivate() || ip.IsLoopback())
}

// ServiceDialer validates the resolved endpoint before a plaintext connection can carry credentials.
func ServiceDialer(raw string, timeout, keepAlive time.Duration) *net.Dialer {
	d := &net.Dialer{Timeout: timeout, KeepAlive: keepAlive}
	if strings.HasPrefix(raw, "http://") {
		d.ControlContext = func(_ context.Context, _, address string, _ syscall.RawConn) error {
			host, _, err := net.SplitHostPort(address)
			if err != nil || !privateServiceIP(host) {
				return errors.New("HTTP service must resolve to a private address")
			}
			return nil
		}
	}
	return d
}
