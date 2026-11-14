//go:build integration

package collection

import (
	"io"
	"strconv"
	"testing"
	"time"
	"uuid"

	"github.com/librecosta/gofurry/services/api/internal/database"
	"github.com/librecosta/gofurry/services/api/internal/database/dbgen"
	"github.com/librecosta/gofurry/services/api/internal/platform"
)

func TestSteamDiscoveryScale(t *testing.T) {
	ctx := t.Context()
	config, _ := fixtureDB(t, "catalog_scale")
	must(t, database.Migrate(ctx, config, "../../migrations", "up", io.Discard))
	pool, err := database.Open(ctx, config)
	must(t, err)
	defer pool.Close()
	q := dbgen.New(pool)
	must(t, q.InitSteamDiscovery(ctx, "scale"))
	d := &Discovery{pool: pool, config: platform.DiscoveryConfig{Namespace: "scale", PollInterval: time.Second, ScanInterval: time.Minute, PageSize: 64, ExplorePerDay: 2}, intake: NewIntake(pool, nil)}
	conn, err := pool.Acquire(ctx)
	must(t, err)
	defer conn.Release()
	previous := 0
	var total int64
	const day int64 = 20000
	for _, size := range []int{100, 1000, 10000} {
		for start := previous; start < size; start += 256 {
			keys, ids := make([]string, 0, 256), make([]uuid.UUID, 0, 256)
			for i := start; i < min(start+256, size); i++ {
				keys = append(keys, strconv.Itoa(i+1))
				ids = append(ids, uuid.NewV7())
			}
			execSQL(t, pool, `INSERT INTO game_intake_candidates (id,platform_code,external_key,discovered_via,discovery_modified,state,reason)
    SELECT id,'steam',external_key,'discovery',100000,'rejected','irrelevant_game' FROM unnest($1::uuid[],$2::text[]) a(id,external_key)`, ids, keys)
		}
		start := time.Now()
		for offset := 0; offset < size; offset += 64 {
			rows := make([]catalogRow, 0, 64)
			for i := offset; i < min(size, offset+64); i++ {
				rows = append(rows, catalogRow{ID: uint32(i + 1), Name: "Fox title", Modified: 100000})
			}
			state, err := q.GetSteamDiscovery(ctx, "scale")
			must(t, err)
			must(t, d.commitPage(ctx, conn, state, []discoveryPick{}, discoveryPage{Apps: rows, More: offset+64 < size, Last: rows[len(rows)-1].ID}, day))
		}
		elapsed := time.Since(start)
		state, err := d.Status(ctx)
		must(t, err)
		total += int64(size)
		if state.Pending != 0 || state.Rows != total {
			t.Fatalf("overlap changed discovery: %+v", state)
		}
		bytes := scalar[int64](t, pool, "SELECT pg_indexes_size('game_intake_candidates')")
		t.Logf("catalog rows=%d page commits=%d duration=%s candidate indexes=%d B", size, (size+63)/64, elapsed, bytes)
		previous = size
	}
}
