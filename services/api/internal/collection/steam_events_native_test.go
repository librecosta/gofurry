//go:build integration

package collection

import (
	"flag"
	"strings"
	"testing"
	"time"
	"uuid"

	"github.com/librecosta/gofurry/services/api/internal/database"
	"github.com/librecosta/gofurry/services/api/internal/database/dbgen"
	"github.com/librecosta/gofurry/services/api/internal/platform"
)

var steamEventNativeConfig = flag.String("steam-events-native-config", "", "explicit isolated Store Events Consumer database YAML")

func TestSteamEventNativeVersioning(t *testing.T) {
	if *steamEventNativeConfig == "" {
		t.Skip("requires an isolated Consumer database containing one verified Steam event")
	}
	cfg, err := platform.LoadConfig(*steamEventNativeConfig)
	must(t, err)
	if cfg.Database == nil || !strings.HasSuffix(cfg.Database.Name, "_test") {
		t.Fatal("requires an isolated *_test database")
	}
	ctx := t.Context()
	pool, err := database.Open(ctx, *cfg.Database)
	must(t, err)
	defer pool.Close()
	tx, err := pool.Begin(ctx)
	must(t, err)
	defer tx.Rollback(ctx)
	q := dbgen.New(tx)
	var row dbgen.EligibleSteamEventObservationsRow
	var id uuid.UUID
	var gid, announcement, public string
	var language int
	must(t, tx.QueryRow(ctx, `SELECT e.id,e.game_source_id,e.collection_source_id,e.binding_id,e.target_id,
		e.namespace,e.scope_key,e.requested_locale,e.egress_region,e.observation_id,e.content_hash,
		e.observed_at,e.confirmed_at,e.event_gid,e.announcement_gid,e.language,g.public_id FROM game_steam_events e
		JOIN game_sources s ON s.id=e.game_source_id JOIN games g ON g.id=s.game_id WHERE e.eligible LIMIT 1`).
		Scan(&id, &row.GameSourceID, &row.CollectionSourceID, &row.BindingID, &row.TargetID, &row.Namespace, &row.ScopeKey,
			&row.RequestedLocale, &row.EgressRegion, &row.ObservationID, &row.ContentHash, &row.ObservedAt, &row.ConfirmedAt,
			&gid, &announcement, &language, &public))
	versions := func() int {
		var count int
		must(t, tx.QueryRow(ctx, "SELECT count(*) FROM game_steam_event_versions WHERE event_id=$1", id).Scan(&count))
		return count
	}
	advance := func() {
		next := uuid.NewV7()
		row.ObservationID = &next
		row.ObservedAt.Time = row.ObservedAt.Time.Add(time.Second)
		row.ConfirmedAt.Time = row.ConfirmedAt.Time.Add(time.Second)
	}
	before := versions()
	now := time.Now().Unix()
	published := true
	votes := 20
	event := steamEvent{EventGID: gid, AnnouncementGID: announcement, Headline: "R3 version test", Body: "[h2]First[/h2]",
		Language: &language, Published: &published, Post: &now, Update: &now, VotesUp: &votes}
	advance()
	must(t, saveSteamEvent(ctx, q, row, steamEventPage{}, event))
	if versions() != before+1 {
		t.Fatal("first new body did not receive one version")
	}
	advance()
	votes = 21
	must(t, saveSteamEvent(ctx, q, row, steamEventPage{}, event))
	var got int
	must(t, tx.QueryRow(ctx, "SELECT (metrics->>'vote_up_count')::int FROM game_steam_events WHERE id=$1", id).Scan(&got))
	if versions() != before+1 || got != 21 {
		t.Fatalf("metrics-only update created a version or lost its count: versions=%d, count=%d", versions(), got)
	}
	event.VotesUp = nil
	event.Body = "[h2]Revised[/h2]"
	now++
	advance()
	must(t, saveSteamEvent(ctx, q, row, steamEventPage{}, event))
	must(t, tx.QueryRow(ctx, "SELECT (metrics->>'vote_up_count')::int FROM game_steam_events WHERE id=$1", id).Scan(&got))
	detail, err := q.SteamEventOperationsDetail(ctx, dbgen.SteamEventOperationsDetailParams{PublicID: public, ID: id})
	must(t, err)
	if versions() != before+2 || got != 21 || !strings.Contains(string(detail.Previous), "First") ||
		!strings.Contains(detail.Html, "Revised") || detail.ObservationID != *row.ObservationID {
		t.Fatalf("revised body, latest provenance, previous revision, or known counter missing")
	}
}
