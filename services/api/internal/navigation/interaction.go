package navigation

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/netip"
	"strconv"
	"time"

	"github.com/librecosta/gofurry/services/api/internal/catalog"
	"github.com/librecosta/gofurry/services/api/internal/database/dbgen"
	"github.com/redis/go-redis/v9"
)

const DetailView = "detail_view"
const OutboundClick = "outbound_click"

var ErrLimited = errors.New("navigation_interaction_limit")

type Counter struct {
	service *Service
	redis   *redis.Client
	secret  []byte
	prefix  string
}

func NewCounter(service *Service, client *redis.Client, secret []byte, region string) *Counter {
	return &Counter{service, client, append([]byte(nil), secret...), "gofurry:v1:{" + region + "}:nav:"}
}

func (c *Counter) Enabled() bool {
	return c.redis != nil && len(c.secret) == 32 && c.service.pool != nil
}

func (c *Counter) key(kind, data string) string {
	mac := hmac.New(sha256.New, c.secret)
	_, _ = mac.Write([]byte(c.prefix + kind + ":" + data))
	return c.prefix + kind + ":" + hex.EncodeToString(mac.Sum(nil))
}

// Reserve both attempt budgets first, then apply the per-Site rolling gate.
// Both half-hour keys are checked at the exact timestamp, including across UTC midnight.
var reserve = redis.NewScript(`
local function attempt(key, max, ttl)
 local n=tonumber(redis.call('GET',key) or '0')
 if n>=max then return false end
 if redis.call('INCR',key)==1 then redis.call('EXPIRE',key,ttl) end
 return true
end
if not attempt(KEYS[1],600,120) then return 2 end
if not attempt(KEYS[2],30,120) then return 2 end
local now=tonumber(ARGV[1])
for i=3,4 do
 local prior=tonumber(redis.call('GET',KEYS[i]) or '0')
 if prior>0 and now-prior<1800 then return 0 end
end
if not attempt(KEYS[5],8,172800) then return 2 end
redis.call('SET',KEYS[3],ARGV[1],'EX',3600)
return 1`)

func (c *Counter) Count(ctx context.Context, ip netip.Addr, id, kind string) (bool, error) {
	if !catalog.ValidPublicID(id) {
		return false, catalog.ErrNotFound
	}
	if kind != DetailView && kind != OutboundClick || !ip.IsValid() || ip.Zone() != "" || ip.IsUnspecified() || ip.IsMulticast() {
		return false, catalog.ErrInvalid
	}
	if !c.Enabled() {
		return false, catalog.ErrUnavailable
	}
	now := c.service.now().UTC()
	minute := strconv.FormatInt(now.Unix()/60, 10)
	bucket := now.Unix() / 1800
	address := ip.Unmap().String()
	event := address + "|" + id + "|" + kind + "|"
	keys := []string{
		c.prefix + "minute:" + minute,
		c.key("ip", address+"|"+minute),
		c.key("half", event+strconv.FormatInt(bucket, 10)),
		c.key("half", event+strconv.FormatInt(bucket-1, 10)),
		c.key("day", event+now.Format(time.DateOnly)),
	}
	gate, cancel := context.WithTimeout(ctx, 250*time.Millisecond)
	accepted, err := reserve.Run(gate, c.redis, keys, now.Unix()).Int()
	cancel()
	if err != nil {
		return false, catalog.ErrUnavailable
	}
	switch accepted {
	case 0:
		return false, nil
	case 2:
		return false, ErrLimited
	case 1:
	default:
		return false, catalog.ErrUnavailable
	}
	// A failed or ambiguous write is never retried after reservation.
	detail, outbound := int32(0), int32(0)
	if kind == DetailView {
		detail = 1
	} else {
		outbound = 1
	}
	ctx, stop := context.WithTimeout(ctx, 800*time.Millisecond)
	defer stop()
	n, err := dbgen.New(c.service.pool).CountNavigationInteraction(ctx, dbgen.CountNavigationInteractionParams{
		UtcDay: pgDay(now), Detail: detail, Outbound: outbound, PublicID: id,
	})
	if err != nil {
		return false, catalog.ErrUnavailable
	}
	if n == 0 {
		return false, catalog.ErrNotFound
	}
	return true, nil
}
