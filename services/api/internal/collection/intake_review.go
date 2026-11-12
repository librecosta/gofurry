package collection

import (
	"context"
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"errors"
	"regexp"
	"time"
	"uuid"

	"github.com/jackc/pgx/v5"
	"github.com/librecosta/gofurry/services/api/internal/catalog"
	"github.com/librecosta/gofurry/services/api/internal/database/dbgen"
)

var intakeEvidenceRef = regexp.MustCompile(`^[a-z][a-z0-9_.-]{2,95}$`)

type CandidateStatus struct {
	ID          uuid.UUID       `json:"id,omitzero"`
	Platform    string          `json:"platform,omitempty"`
	Key         string          `json:"key,omitempty"`
	Via         string          `json:"discovered_via,omitempty"`
	State       string          `json:"state,omitempty"`
	Reason      string          `json:"reason,omitempty"`
	NextAt      *time.Time      `json:"next_at,omitempty"`
	Target      *uuid.UUID      `json:"target_id,omitempty"`
	Job         *uuid.UUID      `json:"job_id,omitempty"`
	Attempt     *uuid.UUID      `json:"attempt_id,omitempty"`
	Observation *uuid.UUID      `json:"observation_id,omitempty"`
	Game        *uuid.UUID      `json:"game_id,omitempty"`
	Source      *uuid.UUID      `json:"game_source_id,omitempty"`
	PublicID    string          `json:"public_id,omitempty"`
	GameState   string          `json:"game_state,omitempty"`
	Version     int32           `json:"decision_version,omitempty"`
	Evidence    *IntakeEvidence `json:"evidence,omitempty"`
}

type IntakeEvidence struct {
	ObservationHash string         `json:"observation_hash,omitempty"`
	ScopeKey        string         `json:"scope_key,omitempty"`
	Review          jsontext.Value `json:"review,omitzero"`
	ReviewUntil     *time.Time     `json:"review_until,omitempty"`
	Relation        jsontext.Value `json:"relation,omitzero"`
}

func candidateStatus(c dbgen.GameIntakeCandidate) CandidateStatus {
	out := CandidateStatus{
		ID: c.ID, Platform: c.PlatformCode, Key: c.ExternalKey, Via: c.DiscoveredVia, State: c.State, Reason: c.Reason,
		Target: c.TargetID, Job: c.JobID, Attempt: c.AttemptID, Observation: c.ObservationID, Game: c.GameID, Source: c.GameSourceID, Version: c.DecisionVersion,
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
	if row.ObservationHash != "" || string(row.Review) != "{}" || string(row.Relation) != "{}" {
		out.Evidence = &IntakeEvidence{ObservationHash: row.ObservationHash, ScopeKey: row.ProfileScopeKey}
		if string(row.Review) != "{}" {
			out.Evidence.Review = row.Review
		}
		if row.ReviewUntil.Valid {
			out.Evidence.ReviewUntil = &row.ReviewUntil.Time
		}
		if string(row.Relation) != "{}" {
			out.Evidence.Relation = row.Relation
		}
	}
	if row.GameID == nil {
		return out, nil
	}
	game, err := s.queries.IntakeGameStatus(ctx, *row.GameID)
	out.PublicID, out.GameState = game.PublicID, game.GameStatus
	return out, err
}

func (s *Intake) Status(ctx context.Context) ([]dbgen.IntakeStatusRow, error) {
	return s.queries.IntakeStatus(ctx)
}

type positiveReview struct {
	FurryRef string    `json:"furry_ref"`
	SFWRef   string    `json:"sfw_ref"`
	At       time.Time `json:"reviewed_at"`
	Policy   string    `json:"policy"`
}

// ReviewSteam accepts evidence from a reviewed work synopsis and a separate
// content assessment. References name records in the operator's audited
// source, and cannot be supplied by a Storefront response.
func (s *Intake) ReviewSteam(ctx context.Context, key, furryRef, sfwRef string) error {
	source, err := intakeSource("steam", key)
	if err != nil || !evidencePair(furryRef, sfwRef) {
		return ErrIntakeInput
	}
	proof, _ := json.Marshal(positiveReview{furryRef, sfwRef, time.Now().UTC(), "furry-and-sfw-1"})
	return s.transact(ctx, func(ctx context.Context, q *dbgen.Queries) error {
		c, err := lockIntakeCandidate(ctx, q, source)
		if err != nil {
			return err
		}
		if c.State == "rejected" || c.Reason == "merge_suggested" {
			return ErrIntakeState
		}
		if err = q.PutIntakeReview(ctx, dbgen.PutIntakeReviewParams{ID: c.ID, Review: proof}); err != nil || c.State != "accepted" {
			return err
		}
		game, err := q.LockGame(ctx, *c.GameID)
		if err != nil {
			return err
		}
		return publishIntake(ctx, q, c.ID, game)
	})
}

func evidencePair(a, b string) bool {
	return a != b && intakeEvidenceRef.MatchString(a) && intakeEvidenceRef.MatchString(b)
}

func (s *Intake) RevokeSteamReview(ctx context.Context, key string) error {
	source, err := intakeSource("steam", key)
	if err != nil {
		return err
	}
	return s.transact(ctx, func(ctx context.Context, q *dbgen.Queries) error {
		c, err := lockIntakeCandidate(ctx, q, source)
		if err != nil {
			return err
		}
		// The public read gate takes effect in this same commit and leaves
		// the original decision and first publication time available.
		return q.RevokeIntakeReview(ctx, c.ID)
	})
}

type relationProof struct {
	Platform   string    `json:"platform"`
	Key        string    `json:"key"`
	ForwardURL string    `json:"forward_url"`
	ReverseURL string    `json:"reverse_url"`
	ForwardRef string    `json:"forward_ref"`
	ReverseRef string    `json:"reverse_ref"`
	Kind       string    `json:"kind"`
	At         time.Time `json:"reviewed_at"`
	Rule       int       `json:"rule"`
}

func (p relationProof) valid(self, other catalog.Source) bool {
	return self.PlatformCode != other.PlatformCode && p.Platform == other.PlatformCode && p.Key == other.ExternalKey &&
		p.ForwardURL == other.SourceURL && p.ReverseURL == self.SourceURL &&
		evidencePair(p.ForwardRef, p.ReverseRef) && p.Kind == "reviewed_reciprocal_work_links" && p.Rule == 1 && !p.At.IsZero()
}

// RecordRelation consumes a reviewed assertion of reciprocal, work-specific
// links and matching full-game type. It never follows the links itself.
func (s *Intake) RecordRelation(ctx context.Context, platform, key, otherPlatform, otherKey, forward, reverse string) error {
	source, err := intakeSource(platform, key)
	other, otherErr := intakeSource(otherPlatform, otherKey)
	if err != nil || otherErr != nil {
		return ErrIntakeInput
	}
	proof := relationProof{other.PlatformCode, other.ExternalKey, other.SourceURL, source.SourceURL,
		forward, reverse, "reviewed_reciprocal_work_links", time.Now().UTC(), 1}
	if !proof.valid(source, other) {
		return ErrIntakeInput
	}
	data, _ := json.Marshal(proof)
	return s.transact(ctx, func(ctx context.Context, q *dbgen.Queries) error {
		if _, err := s.submit(ctx, q, source, "reviewed_link"); err != nil {
			return err
		}
		c, err := lockIntakeCandidate(ctx, q, source)
		if err != nil || c.State == "rejected" {
			return errors.Join(err, ErrIntakeState)
		}
		var previous relationProof
		if json.Unmarshal(c.Relation, &previous) != nil {
			return ErrIntakeState
		}
		if previous.Platform != "" && (previous.Platform != proof.Platform || previous.Key != proof.Key) {
			return ErrIntakeState
		}
		if err = q.SaveIntakeRelation(ctx, dbgen.SaveIntakeRelationParams{ID: c.ID, Relation: data}); err != nil {
			return err
		}
		if c.State == "accepted" {
			linked, err := findIntakeSource(ctx, q, other)
			if err != nil {
				return err
			}
			if linked != nil && linked.Active && linked.SourceUrl == other.SourceURL && linked.GameID != *c.GameID {
				return q.NoteIntakeConflict(ctx, c.ID)
			}
		}
		if c.State == "deferred" && (c.Reason == "weak_identity_match" || c.Reason == "related_source_unavailable") {
			return q.WakeIntake(ctx, c.ID)
		}
		return nil
	})
}

func (s *Intake) RevokeRelation(ctx context.Context, platform, key string) error {
	source, err := intakeSource(platform, key)
	if err != nil {
		return err
	}
	return s.transact(ctx, func(ctx context.Context, q *dbgen.Queries) error {
		c, err := lockIntakeCandidate(ctx, q, source)
		if err != nil {
			return err
		}
		if c.State != "accepted" || !c.RelationAttached {
			return ErrIntakeState
		}
		game, err := q.LockGame(ctx, *c.GameID)
		if err != nil {
			return err
		}
		if err = q.DeactivateIntakeSource(ctx, dbgen.DeactivateIntakeSourceParams{ID: *c.GameSourceID, GameID: game.ID}); err != nil {
			return err
		}
		if err = q.CloseRemovedGameBindings(ctx, &game.ID); err != nil {
			return err
		}
		if _, err = q.UpdateGame(ctx, dbgen.UpdateGameParams{ID: game.ID, Slug: game.Slug}); err != nil {
			return err
		}
		return q.RevokeIntakeRelation(ctx, c.ID)
	})
}
