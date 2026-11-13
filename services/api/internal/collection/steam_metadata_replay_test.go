//go:build integration

package collection

import (
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"testing"
	"time"
	"uuid"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/librecosta/gofurry/services/api/internal/catalog"
	"github.com/librecosta/gofurry/services/api/internal/database/dbgen"
)

func checkSteamWithdrawal(t *testing.T, pool *pgxpool.Pool, svc *Service, consumer dbgen.CollectionConsumer, gameID string, source uuid.UUID) {
	t.Helper()
	ctx := t.Context()
	tx, err := pool.Begin(ctx)
	must(t, err)
	defer tx.Rollback(ctx)
	q := dbgen.New(tx)
	var prior dbgen.CollectionCurrent
	must(t, tx.QueryRow(ctx, `SELECT scope_key,scope,position,content_hash,observation_id,observed_at,received_at,data
 FROM collection_current WHERE generation=$1 AND source_id=$2 AND capability='steam.details'
 AND scope->>'locale'='en' AND scope->>'region'='us' AND scope#>>'{dimensions,section}'='details'`, consumer.Generation, source).
		Scan(&prior.ScopeKey, &prior.Scope, &prior.Position, &prior.ContentHash, &prior.ObservationID, &prior.ObservedAt, &prior.ReceivedAt, &prior.Data))
	var scope segmentScope
	must(t, json.Unmarshal(prior.Scope, &scope))
	var target uuid.UUID
	must(t, tx.QueryRow(ctx, "SELECT target_id FROM collection_sources WHERE id=$1", source).Scan(&target))
	ref := identity{TargetID: target, Capability: intakeCapability, Schema: 1, ScopeKey: prior.ScopeKey}
	now := time.Now().UTC()
	old := observation{identity: ref, ID: *prior.ObservationID, Scope: scope, State: "present", ObservedAt: prior.ObservedAt.Time,
		ReceivedAt: prior.ReceivedAt.Time, Data: jsontext.Value(prior.Data), Hash: prior.ContentHash, Source: "steam"}
	base := event{identity: ref, Position: prior.Position, Action: "upsert", Committed: now, Observation: &old}
	update := old
	update.ID, update.ObservedAt, update.ReceivedAt = uuid.NewV7(), now, now
	update.Data = jsontext.Value(`{"app_id":42,"tier":"observed_storefront","source_url":"https://store.steampowered.com/api/appdetails","name":"Partial detail","type":"game"}`)
	partial := event{identity: ref, Position: prior.Position + 1, Action: "upsert", Committed: now, Observation: &update}
	bad := update
	bad.Data = jsontext.Value(`{"app_id":43,"tier":"observed_storefront","source_url":"https://store.steampowered.com/api/appdetails","name":"Foreign detail"}`)
	malformed := event{identity: ref, Position: prior.Position + 2, Action: "upsert", Committed: now, Observation: &bad}
	withdrawal := event{identity: ref, Position: prior.Position + 3, Action: "withdraw", Committed: now, Tombstone: scope}
	must(t, svc.applyPage(ctx, q, *consumer.Epoch, *consumer.Generation, []event{partial, malformed, withdrawal}, false))
	must(t, svc.applyPage(ctx, q, *consumer.Epoch, *consumer.Generation, []event{base, partial, withdrawal}, false))
	// A snapshot in another epoch may contain the old material. Its position
	// is larger, but its original receipt is earlier than the withdrawal.
	newGeneration := uuid.NewV7()
	olderEpoch := uuid.NewV7()
	base.Position += 1000
	must(t, svc.applyPage(ctx, q, olderEpoch, newGeneration, []event{base}, true))
	must(t, saveSteamMetadata(ctx, q, newGeneration, olderEpoch, &source, ref.ScopeKey))
	var closed dbgen.CollectionBinding
	must(t, tx.QueryRow(ctx, `UPDATE collection_bindings SET effective_until=now()
 WHERE id=(SELECT binding_id FROM game_source_metadata WHERE scope_key=$1) RETURNING id,game_id,game_source_id`, ref.ScopeKey).
		Scan(&closed.ID, &closed.GameID, &closed.GameSourceID))
	// Retention keeps old Steam news; it is not a current measurement.
	_, err = tx.Exec(ctx, "UPDATE game_evidence_news SET published_at=now()-interval '365 days' WHERE source_id=$1", source)
	must(t, err)
	must(t, q.PruneGameEvidencePlayers(ctx))
	row, err := q.ReadGamePresentation(ctx, dbgen.ReadGamePresentationParams{PublicID: gameID, EgressRegion: "cn"})
	must(t, err)
	var sources []catalog.GameSourceSnapshot
	must(t, json.Unmarshal(row.Sources, &sources))
	if len(sources[1].News) != 1 {
		t.Fatal("historic news was dropped by retention")
	}
	retained := false
	for _, m := range sources[1].Metadata {
		if m.ScopeKey == ref.ScopeKey && m.State == "historical" && m.ObservationID == old.ID &&
			m.Details != nil && m.Details.Description == "An independent English description." {
			retained = true
		}
	}
	if !retained {
		t.Fatal("partial, malformed, withdrawal or older-epoch replay erased last successful metadata")
	}
	newBinding, err := q.AddCollectionGameBinding(ctx, dbgen.AddCollectionGameBindingParams{
		ID: uuid.NewV7(), SourceID: source, GameID: closed.GameID, GameSourceID: closed.GameSourceID,
	})
	must(t, err)
	_, err = tx.Exec(ctx, `INSERT INTO collection_plan_bindings (plan_id,binding_id,source_id)
 SELECT plan_id,$1,source_id FROM collection_plan_bindings WHERE binding_id=$2`, newBinding.ID, closed.ID)
	must(t, err)
	// An old observation confirmed again after rebinding cannot be attributed
	// to the new period. A fresh result for that binding can.
	replayed := result{observation: old, Position: withdrawal.Position + 1, Confirmed: now.Add(time.Second)}
	must(t, svc.project(ctx, q, *consumer.Generation, *consumer.Epoch, replayed, true))
	must(t, saveSteamMetadata(ctx, q, *consumer.Generation, *consumer.Epoch, &source, ref.ScopeKey))
	var bindingID, observationID uuid.UUID
	readIDs := func() {
		must(t, tx.QueryRow(ctx, "SELECT binding_id,observation_id FROM game_source_metadata WHERE scope_key=$1", ref.ScopeKey).
			Scan(&bindingID, &observationID))
	}
	readIDs()
	if bindingID != closed.ID || observationID != old.ID {
		t.Fatal("old evidence was relabeled as belonging to a new binding")
	}
	newTime := now.Add(2 * time.Second)
	update.ID, update.ObservedAt, update.ReceivedAt = uuid.NewV7(), newTime, newTime
	update.Data = jsontext.Value(`{"app_id":42,"tier":"observed_storefront","source_url":"https://store.steampowered.com/api/appdetails","name":"New bound details","type":"game","description":"A newly confirmed Storefront description."}`)
	newResult := result{observation: update, Position: withdrawal.Position + 2, Confirmed: newTime}
	must(t, svc.project(ctx, q, *consumer.Generation, *consumer.Epoch, newResult, false))
	readIDs()
	if bindingID != newBinding.ID || observationID != update.ID {
		t.Fatal("fresh evidence was not attributed to the new binding")
	}
}
