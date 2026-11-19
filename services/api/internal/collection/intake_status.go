package collection

import (
	"context"
	"errors"
	"time"
	"uuid"

	"github.com/jackc/pgx/v5"
	"github.com/librecosta/gofurry/services/api/internal/database/dbgen"
)

type CandidateStatus struct {
	ID           uuid.UUID       `json:"id,omitzero"`
	Platform     string          `json:"platform,omitempty"`
	Key          string          `json:"key,omitempty"`
	Via          string          `json:"discovered_via,omitempty"`
	State        string          `json:"state,omitempty"`
	Reason       string          `json:"reason,omitempty"`
	NextAt       *time.Time      `json:"next_at,omitempty"`
	Target       *uuid.UUID      `json:"target_id,omitempty"`
	Job          *uuid.UUID      `json:"job_id,omitempty"`
	Attempt      *uuid.UUID      `json:"attempt_id,omitempty"`
	Observation  *uuid.UUID      `json:"observation_id,omitempty"`
	Game         *uuid.UUID      `json:"game_id,omitempty"`
	Source       *uuid.UUID      `json:"game_source_id,omitempty"`
	PublicID     string          `json:"public_id,omitempty"`
	GameState    string          `json:"game_state,omitempty"`
	GameRevision int64           `json:"game_revision,omitempty"`
	Evidence     *IntakeEvidence `json:"evidence,omitempty"`
}

type IntakeEvidence struct {
	ObservationHash string `json:"observation_hash,omitempty"`
	ScopeKey        string `json:"scope_key,omitempty"`
}

func candidateStatus(c dbgen.GameIntakeCandidate) CandidateStatus {
	out := CandidateStatus{
		ID: c.ID, Platform: c.PlatformCode, Key: c.ExternalKey, Via: c.DiscoveredVia, State: c.State, Reason: c.Reason,
		Target: c.TargetID, Job: c.JobID, Attempt: c.AttemptID, Observation: c.ObservationID, Game: c.GameID, Source: c.GameSourceID,
	}
	if c.NextAt.Valid {
		out.NextAt = &c.NextAt.Time
	}
	return out
}

func (s *Intake) Inspect(ctx context.Context, platform, key string) (CandidateStatus, error) {
	source, err := intakeSource(platform, key)
	if err != nil {
		return CandidateStatus{}, ErrIntakeInput
	}
	row, err := s.queries.IntakeCandidate(ctx, dbgen.IntakeCandidateParams{PlatformCode: source.PlatformCode, ExternalKey: source.ExternalKey})
	if errors.Is(err, pgx.ErrNoRows) {
		err = ErrIntakeState
	}
	out := candidateStatus(row)
	if err != nil {
		return out, err
	}
	if row.ObservationHash != "" {
		out.Evidence = &IntakeEvidence{ObservationHash: row.ObservationHash, ScopeKey: row.ProfileScopeKey}
	}
	if row.GameID == nil {
		return out, nil
	}
	game, err := s.queries.IntakeGameStatus(ctx, *row.GameID)
	out.PublicID, out.GameState, out.GameRevision = game.PublicID, game.GameStatus, game.Revision
	return out, err
}

func (s *Intake) Status(ctx context.Context) ([]dbgen.IntakeStatusRow, error) {
	return s.queries.IntakeStatus(ctx)
}
