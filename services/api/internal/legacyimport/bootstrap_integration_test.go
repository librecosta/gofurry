//go:build integration

package legacyimport

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/librecosta/gofurry/services/api/internal/database/dbgen"
)

func TestBootstrapFreshTargetAndExactResume(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	c, _, psql, _ := fixture(t, ctx, t.TempDir())
	conn, err := c.open(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	session, err := conn.Acquire(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Release()
	p := Plan{Backup: c.Backup, Content: c.Content, Decisions: strings.Repeat("b", 64)}
	hash := strings.Repeat("c", 64)
	if err = bootstrapTarget(ctx, session.Conn(), "plan", "", Plan{}); err != nil {
		t.Fatal(err)
	}
	psql(c.Database.Name, "CREATE TABLE unrelated_fixture (id int); INSERT INTO unrelated_fixture VALUES (1);")
	if err = bootstrapTarget(ctx, session.Conn(), "apply", hash, p); err == nil || !strings.Contains(err.Error(), "unrelated data") {
		t.Fatalf("fresh guard missed an unrelated row: %v", err)
	}
	psql(c.Database.Name, "DROP TABLE unrelated_fixture; CREATE ROLE bootstrap_readonly LOGIN PASSWORD 'bootstrap-fixture-read-only'; GRANT USAGE ON SCHEMA public TO bootstrap_readonly; GRANT SELECT ON ALL TABLES IN SCHEMA public TO bootstrap_readonly;")
	readConfig := c
	readConfig.Database.User = "bootstrap_readonly"
	readConfig.Database.Password = "bootstrap-fixture-read-only"
	readPool, err := readConfig.open(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer readPool.Close()
	readSession, err := readPool.Acquire(ctx)
	if err != nil {
		t.Fatal(err)
	}
	err = bootstrapTarget(ctx, readSession.Conn(), "plan", "", Plan{})
	readSession.Release()
	if err == nil || !strings.Contains(err.Error(), "write privileges") {
		t.Fatalf("fresh guard accepted a read-only installer: %v", err)
	}

	q := dbgen.New(session.Conn())
	if err = q.RegisterLegacyImportBatch(ctx, dbgen.RegisterLegacyImportBatchParams{
		PlanSha256: hash, LegacyDeployment: deployment, BackupSha256: p.Backup, ContentSha256: p.Content, DecisionsSha256: p.Decisions,
	}); err != nil {
		t.Fatal(err)
	}
	if err = bootstrapTarget(ctx, session.Conn(), "apply", hash, p); err != nil {
		t.Fatal(err)
	}
	if err = bootstrapTarget(ctx, session.Conn(), "apply", strings.Repeat("d", 64), p); err == nil {
		t.Fatal("different plan accepted a partial target")
	}
	if err = bootstrapTarget(ctx, session.Conn(), "plan", "", Plan{}); err == nil {
		t.Fatal("planning accepted an already started target")
	}
}
