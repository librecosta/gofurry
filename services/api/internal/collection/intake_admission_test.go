//go:build integration

package collection

import (
	"errors"
	"io"
	"strconv"
	"testing"
	"time"

	"github.com/librecosta/gofurry/services/api/internal/database"
)

func TestGameIntakeAdmission(t *testing.T) {
	ctx := t.Context()
	db, _ := fixtureDB(t, "admission")
	must(t, database.Migrate(ctx, db, "../../migrations", "up", io.Discard))
	pool, err := database.Open(ctx, db)
	must(t, err)
	t.Cleanup(pool.Close)
	s := NewIntake(pool, nil)
	ids := make([]CandidateStatus, 10000)
	var previous int
	for _, n := range []int{100, 1000, 10000} {
		insertStart := time.Now()
		for i := previous; i < n; i++ {
			id, err := s.SubmitSteam(ctx, strconv.Itoa(i+1), "operator")
			must(t, err)
			ids[i] = id
		}
		insertTime := time.Since(insertStart)
		duplicateStart := time.Now()
		for i := range n {
			row, err := s.SubmitSteam(ctx, "000"+strconv.Itoa(i+1), "discovery")
			must(t, err)
			if row.ID != ids[i].ID {
				t.Fatal("duplicate submission returned a new identity")
			}
		}
		elapsed := time.Since(duplicateStart)
		count(t, pool, int64(n), "SELECT count(*) FROM game_intake_candidates")
		heap := scalar[int64](t, pool, "SELECT pg_table_size('game_intake_candidates')")
		index := scalar[int64](t, pool, "SELECT pg_indexes_size('game_intake_candidates')")
		t.Logf("%d candidates: %d new in %s; %d duplicates in %s; table %d B indexes %d B", n, n-previous,
			insertTime.Round(time.Millisecond), n, elapsed.Round(time.Millisecond), heap, index)
		previous = n
	}
	if _, err := s.SubmitSteam(ctx, "10001", "discovery"); !errors.Is(err, ErrIntakeCapacity) {
		t.Fatal("backlog cap was not enforced")
	}
	must(t, s.Dismiss(ctx, "steam", "1"))
	again, err := s.SubmitSteam(ctx, "0001", "discovery")
	must(t, err)
	if again.ID != ids[0].ID || again.State != "rejected" {
		t.Fatal("dismissal reopened a previously rejected key")
	}
	_, err = s.SubmitSteam(ctx, "10001", "operator")
	must(t, err)
	count(t, pool, 10000, "SELECT count(*) FROM game_intake_candidates WHERE state='discovered'")
	count(t, pool, 0, "SELECT count(*) FROM games")
}
