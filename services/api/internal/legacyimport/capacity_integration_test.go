//go:build integration

package legacyimport

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/librecosta/gofurry/services/api/internal/legacyseed"
)

func testCollectionCapacity(t *testing.T, ctx context.Context, c Config, seed legacyseed.Config, psql func(string, string), pool *pgxpool.Pool, base string, ds []decision) {
	t.Helper()
	psql(seed.GFG.Name, `INSERT INTO gfg_game (id,name_en,appid)
 SELECT n,'Capacity Game '||n,0 FROM generate_series(1000,1512) n;
 INSERT INTO gfg_game_collection (id,code,name_en,status) VALUES
 (10,'capacity-511','Capacity 511','draft'),(11,'capacity-512','Capacity 512','draft'),(12,'capacity-513','Capacity 513','draft');
 INSERT INTO gfg_game_collection_item (collection_id,game_id)
 SELECT c,n FROM (VALUES (10,1510),(11,1511),(12,1512)) caps(c,nmax),LATERAL generate_series(1000,nmax) n;
 INSERT INTO gfg_game_collection_tag (collection_id,tag_id) VALUES (12,1);
 INSERT INTO gfg_game_collection_exclusion (collection_id,game_id) VALUES (12,1000);`)

	seed.Snapshot, seed.BackupSHA = "d5b1-capacity", strings.Repeat("c", 64)
	r, err := legacyseed.Run(ctx, "extract", seed)
	if err != nil {
		t.Fatal(err)
	}
	c.Snapshot, c.Backup, c.Content = seed.Snapshot, seed.BackupSHA, r.ContentSHA
	c.SeedDir, c.DecisionsFile = filepath.Join(seed.Output, seed.Snapshot), filepath.Join(base, "capacity-decisions.json")
	testDecisions(t, c, ds)
	p, preflight, err := Run(ctx, c, Options{Mode: "plan"})
	if err != nil {
		t.Fatal(err)
	}
	planPath := filepath.Join(base, "capacity-plan.json")
	testWrite(t, planPath, p.Bytes())
	_, report, err := Run(ctx, c, Options{"apply", planPath, preflight.PlanSHA})
	if !errors.Is(err, ErrIncomplete) {
		t.Fatalf("513-member collection must prevent completion: %v", err)
	}
	for _, k := range []struct{ id, count int }{{10, 511}, {11, 512}, {12, 0}} {
		expectInt(t, pool, ctx, k.count, `SELECT count(*) FROM game_collection_members WHERE collection_id=
		 (SELECT target_id FROM legacy_editorial_ids WHERE kind='collection' AND legacy_id=$1)`, k.id)
	}
	expectInt(t, pool, ctx, 0, `SELECT count(*) FROM game_collection_tag_rules WHERE collection_id=
	 (SELECT target_id FROM legacy_editorial_ids WHERE kind='collection' AND legacy_id=12)`)
	expectInt(t, pool, ctx, 0, `SELECT count(*) FROM game_collection_exclusions WHERE collection_id=
	 (SELECT target_id FROM legacy_editorial_ids WHERE kind='collection' AND legacy_id=12)`)
	for _, phase := range report.Phases {
		if phase.Source.String() == "gfg_game_collection/12" && phase.Phase == "relations" && phase.Issue == "collection_capacity_exceeded" {
			t.Log("collection 511/512 committed; 513-member manual/rule/exclusion group deferred atomically")
			return
		}
	}
	t.Fatal("missing explicit capacity report")
}
