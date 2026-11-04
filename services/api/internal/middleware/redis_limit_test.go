package middleware

import (
	"context"
	"crypto/rand"
	"flag"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/librecosta/gofurry/services/api/internal/platform"
)

var developmentRedis = flag.String("d1-config", "", "Explicit development YAML for opt-in shared-limit checks")

func TestRedisLimitLive(t *testing.T) {
	if *developmentRedis == "" {
		t.Skip("requires explicit development Redis YAML")
	}
	cfg, err := platform.LoadConfig(*developmentRedis)
	if err != nil {
		t.Fatal("development YAML could not load")
	}
	client := platform.NewRedis(cfg.Redis)
	if client == nil {
		t.Fatal("Redis missing")
	}
	defer client.Close()
	region := "CN-d1-" + rand.Text()
	route := "test-budget"
	var ip string
	defer func() {
		client.Del(context.Background(), platform.RedisKey(region, "rate:"+route, "global"), platform.RedisKey(region, "rate:"+route, ip))
	}()
	apps := []*fiber.App{fiber.New(), fiber.New()}
	for _, app := range apps {
		app.Use(RedisLimit(client, region, route, 3, 10))
		app.Get("/test", func(c fiber.Ctx) error { ip = c.IP(); return c.SendStatus(200) })
	}
	for i := range 4 {
		res, err := apps[i%2].Test(httptest.NewRequest("GET", "/test", nil))
		if err != nil {
			t.Fatal(err)
		}
		res.Body.Close()
		want := 200
		if i == 3 {
			want = 429
		}
		if res.StatusCode != want {
			t.Fatal("shared deployment limit failed")
		}
		if i == 3 && res.Header.Get("Retry-After") != "60" {
			t.Fatal("rate response missing retry")
		}
	}
	if ttl, _ := client.TTL(context.Background(), platform.RedisKey(region, "rate:"+route, "global")).Result(); ttl < 55*time.Second || ttl > time.Minute {
		t.Fatal("shared rate bucket has no bounded TTL")
	}
	// Explicitly different region namespace cannot inherit the CN budget.
	intl := region + "-INTL"
	app := fiber.New()
	app.Use(RedisLimit(client, intl, route, 3, 10))
	app.Get("/test", func(c fiber.Ctx) error { return c.SendStatus(200) })
	res, err := app.Test(httptest.NewRequest("GET", "/test", nil))
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != 200 {
		t.Fatal("rate budget crossed regions")
	}
	client.Del(context.Background(), platform.RedisKey(intl, "rate:"+route, "global"), platform.RedisKey(intl, "rate:"+route, ip))
	offline := platform.NewRedis(cfg.Redis)
	offline.Close()
	fallback := fiber.New()
	fallback.Use(RedisLimit(offline, intl, route, 3, 10))
	fallback.Get("/test", func(c fiber.Ctx) error { return c.SendStatus(200) })
	res, err = fallback.Test(httptest.NewRequest("GET", "/test", nil))
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != 200 {
		t.Fatal("Redis outage broke local-rate fallback")
	}
	t.Log("real shared Redis quota, 60s TTL, regional isolation and outage fallback: passed")
}
