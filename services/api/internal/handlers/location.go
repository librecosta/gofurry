package handlers

import (
	"net/netip"

	"github.com/gofiber/fiber/v3"
	"github.com/librecosta/gofurry/services/api/internal/geoip"
)

func Location(resolver *geoip.Resolver) fiber.Handler {
	return func(c fiber.Ctx) error {
		c.Set("Cache-Control", "private, no-store")
		// No client-supplied IP parameter/header selection: Fiber owns the
		// connection and explicitly configured trusted-proxy contract.
		ip, _ := netip.ParseAddr(c.IP())
		location := resolver.Resolve(ip)
		state := "unavailable"
		if location != nil {
			state = "ready"
		}
		return c.JSON(struct {
			State    string          `json:"state"`
			Source   string          `json:"source"`
			Location *geoip.Location `json:"location"`
		}{State: state, Source: "ip", Location: location})
	}
}
