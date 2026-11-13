package collection

import (
	"context"
	json "encoding/json/v2"
	"errors"
	"regexp"
	"strings"
	"time"
	"uuid"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/librecosta/gofurry/services/api/internal/catalog"
	"github.com/librecosta/gofurry/services/api/internal/database/dbgen"
)

var itchKey = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*/[a-z0-9]+(?:-[a-z0-9]+)*$`)
var errIntakeAmbiguousSource = errors.New("multiple legacy sources share a normalized platform key")

func intakeSource(platform, key string) (catalog.Source, error) {
	platform, key = strings.ToLower(strings.TrimSpace(platform)), strings.TrimSpace(key)
	switch platform {
	case "steam":
		return catalog.NormalizeGameSource(catalog.Source{PlatformCode: platform, ExternalKey: key})
	case "itch", "itch_io":
		key = strings.ToLower(key)
		owner, work, _ := strings.Cut(key, "/")
		if !itchKey.MatchString(key) || len(owner) > 63 || len(work) > 80 {
			return catalog.Source{}, ErrIntakeInput
		}
		return catalog.Source{PlatformCode: "itch_io", ExternalKey: key, SourceURL: "https://" + owner + ".itch.io/" + work}, nil
	default:
		return catalog.Source{}, ErrIntakeInput
	}
}

func lockIntakeCandidate(ctx context.Context, q *dbgen.Queries, source catalog.Source) (dbgen.GameIntakeCandidate, error) {
	row, err := q.LockIntakeCandidate(ctx, dbgen.LockIntakeCandidateParams{PlatformCode: source.PlatformCode, ExternalKey: source.ExternalKey})
	if errors.Is(err, pgx.ErrNoRows) {
		err = ErrIntakeState
	}
	return row, err
}

func (s *Intake) resolve(ctx context.Context, claim dbgen.GameIntakeCandidate, o intakeObservation, scopeKey string) error {
	return s.transact(ctx, func(ctx context.Context, q *dbgen.Queries) error {
		c, err := q.LockIntakeLease(ctx, dbgen.LockIntakeLeaseParams{ID: claim.ID, LeaseID: claim.LeaseID})
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrIntakeState
		}
		if err != nil {
			return err
		}
		source, err := intakeSource(c.PlatformCode, c.ExternalKey)
		if err != nil {
			return err
		}
		decision := dbgen.SaveIntakeDecisionParams{ID: c.ID, Profile: []byte("{}"), Reason: "existing_source"}
		if c.PlatformCode == "steam" {
			_, _, decision.ProfileScopeKey = intakeRequest(c)
		}
		if v7(o.ID) {
			decision.AttemptID, decision.ObservationID, decision.ObservationHash = &o.AttemptID, &o.ID, o.Hash
			decision.ProfileAt, decision.ProfileScopeKey = timestamp(o.ObservedAt), scopeKey
			decision.WorkTitle = strings.ToLower(o.detail.Name)
			decision.Profile, _ = json.Marshal(catalog.Localization{Locale: "en", Name: clean(o.detail.Name, 160), Summary: o.detail.Description})
			if err := q.NoteIntakeObservation(ctx, dbgen.NoteIntakeObservationParams{
				ID: c.ID, AttemptID: decision.AttemptID, ObservationID: decision.ObservationID, ObservationHash: o.Hash,
				ProfileScopeKey: scopeKey, Profile: decision.Profile, ProfileAt: decision.ProfileAt, WorkTitle: decision.WorkTitle,
			}); err != nil {
				return err
			}
		}

		current, err := findIntakeSource(ctx, q, source)
		if errors.Is(err, errIntakeAmbiguousSource) {
			return releaseIntake(ctx, q, c, "deferred", "source_key_conflict", 0, 0)
		}
		if err != nil {
			return err
		}
		other, err := relatedIntakeSource(ctx, q, c)
		if errors.Is(err, errIntakeAmbiguousSource) {
			return releaseIntake(ctx, q, c, "deferred", "source_key_conflict", 0, 0)
		}
		if err != nil {
			return err
		}
		if current != nil {
			if !current.Active {
				return releaseIntake(ctx, q, c, "rejected", "source_inactive", 0, 0)
			}
			if current.SourceUrl != source.SourceURL {
				return releaseIntake(ctx, q, c, "deferred", "source_url_unverified", 0, 0)
			}
			if other != nil && other.GameID != current.GameID {
				decision.Reason = "merge_suggested"
			}
			if o.detail.Type != "game" {
				decision.ObservationID, decision.Profile = nil, []byte("{}")
			}
		} else {
			if c.PlatformCode == "steam" {
				switch o.detail.Type {
				case "game":
				case "demo", "dlc", "music", "soundtrack", "video", "tool", "application", "bundle", "advertising", "episode", "series", "mod":
					return releaseIntake(ctx, q, c, "rejected", "not_full_game", 0, 0)
				default:
					return releaseIntake(ctx, q, c, "deferred", "unknown_app_type", 0, 0)
				}
			}
			var gameID uuid.UUID
			switch {
			case other != nil:
				gameID, decision.Reason, decision.RelationAttached = other.GameID, "related_source", true
			case string(c.Relation) != "{}" || c.PlatformCode != "steam":
				return releaseIntake(ctx, q, c, "deferred", "related_source_unavailable", 0, 0)
			default:
				similar, err := q.IntakeTitleConflict(ctx, decision.WorkTitle)
				if err != nil {
					return err
				}
				if similar {
					return releaseIntake(ctx, q, c, "deferred", "weak_identity_match", 0, 0)
				}
				game, err := s.catalog.CreateIntakeGame(ctx, q, o.detail.Name)
				if err != nil {
					return err
				}
				gameID, decision.Reason = game.ID, "created_game"
			}
			game, err := q.LockGame(ctx, gameID)
			if err != nil {
				return err
			}
			if game.Status == catalog.Archived {
				return releaseIntake(ctx, q, c, "deferred", "game_archived", 0, 0)
			}
			if other != nil {
				// Lock the existing Game first, then recheck the reciprocal
				// identity after any concurrent editorial source removal.
				confirmed, err := relatedIntakeSource(ctx, q, c)
				if err != nil {
					return err
				}
				if confirmed == nil || confirmed.GameID != gameID || confirmed.ID != other.ID {
					return releaseIntake(ctx, q, c, "deferred", "related_source_unavailable", 0, 0)
				}
				room, err := q.IntakeSourceCapacity(ctx, gameID)
				if err != nil {
					return err
				}
				if !room {
					return releaseIntake(ctx, q, c, "deferred", "source_capacity", 0, 0)
				}
			}
			inserted, err := q.InsertIntakeSource(ctx, dbgen.InsertIntakeSourceParams{
				ID: uuid.NewV7(), GameID: gameID, PlatformCode: source.PlatformCode, ExternalKey: source.ExternalKey, SourceUrl: source.SourceURL,
			})
			if errors.Is(err, pgx.ErrNoRows) {
				// A catalog writer won the unique source key. Roll back any
				// new Game and re-read ownership on the next leased attempt.
				return ErrIntakeState
			}
			if err != nil {
				return err
			}
			current = &inserted
		}
		game, err := q.LockGame(ctx, current.GameID)
		if err != nil {
			return err
		}
		if game.Status == catalog.Archived {
			return releaseIntake(ctx, q, c, "deferred", "game_archived", 0, 0)
		}
		confirmed, err := findIntakeSource(ctx, q, source)
		if err != nil {
			return err
		}
		if confirmed == nil || !confirmed.Active || confirmed.ID != current.ID || confirmed.GameID != current.GameID {
			return ErrIntakeState
		}
		decision.GameID, decision.GameSourceID = &game.ID, &current.ID
		if err := q.SaveIntakeDecision(ctx, decision); err != nil {
			return err
		}
		if decision.RelationAttached {
			game, err = q.UpdateGame(ctx, dbgen.UpdateGameParams{ID: game.ID, Slug: game.Slug})
			if err != nil {
				return err
			}
		}
		if c.PlatformCode == "steam" {
			if err := q.PickIntakeSource(ctx, dbgen.PickIntakeSourceParams{ID: game.ID, IntakeDisplaySource: &current.ID}); err != nil {
				return err
			}
			if err := intakeSchedule(ctx, q, c, *current); err != nil {
				return err
			}
		}
		if err := q.WakeRelatedIntake(ctx, dbgen.WakeRelatedIntakeParams{Platform: c.PlatformCode, SourceKey: c.ExternalKey}); err != nil {
			return err
		}
		if decision.Reason != "merge_suggested" {
			return publishIntake(ctx, q, c.ID, game)
		}
		return nil
	})
}

func findIntakeSource(ctx context.Context, q *dbgen.Queries, source catalog.Source) (*dbgen.GameSource, error) {
	if source.PlatformCode == "itch_io" {
		rows, err := q.ExistingIntakeItchSources(ctx, source.ExternalKey)
		if err != nil || len(rows) == 0 {
			return nil, err
		}
		if len(rows) != 1 {
			return nil, errIntakeAmbiguousSource
		}
		return &rows[0], nil
	}
	value, err := q.ExistingIntakeSource(ctx, dbgen.ExistingIntakeSourceParams{PlatformCode: source.PlatformCode, ExternalKey: source.ExternalKey})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	return &value, err
}

func relatedIntakeSource(ctx context.Context, q *dbgen.Queries, c dbgen.GameIntakeCandidate) (*dbgen.GameSource, error) {
	var proof relationProof
	if err := json.Unmarshal(c.Relation, &proof); err != nil {
		return nil, ErrIntakeState
	}
	if proof.Platform == "" {
		return nil, nil
	}
	other, err := intakeSource(proof.Platform, proof.Key)
	self, selfErr := intakeSource(c.PlatformCode, c.ExternalKey)
	if err != nil || selfErr != nil || !proof.valid(self, other) {
		return nil, ErrIntakeState
	}
	row, err := findIntakeSource(ctx, q, other)
	if err != nil || row == nil || !row.Active {
		return nil, err
	}
	if row.SourceUrl != other.SourceURL {
		return nil, nil
	}
	return row, nil
}

func intakeSchedule(ctx context.Context, q *dbgen.Queries, c dbgen.GameIntakeCandidate, gameSource dbgen.GameSource) error {
	source, err := q.SaveCollectionSource(ctx, dbgen.SaveCollectionSourceParams{
		ID: uuid.NewV7(), Namespace: c.Namespace, Provider: "steam", ResourceType: "app", ExternalKey: c.ExternalKey,
	})
	if err != nil {
		return err
	}
	if c.TargetID != nil {
		if source.TargetID != nil && *source.TargetID != *c.TargetID {
			return errRemoteConflict
		}
		if err = q.SaveCollectionTarget(ctx, dbgen.SaveCollectionTargetParams{ID: source.ID, TargetID: c.TargetID}); err != nil {
			return err
		}
	}
	binding, err := q.AddCollectionGameBinding(ctx, dbgen.AddCollectionGameBindingParams{
		ID: uuid.NewV7(), SourceID: source.ID, GameID: &gameSource.GameID, GameSourceID: &gameSource.ID,
	})
	if err != nil {
		return err
	}
	plan := intakePlan(c.EgressRegion)
	params, scope, policy, err := plan.params("app", c.ExternalKey)
	if err != nil {
		return err
	}
	id, err := q.FindIntakePlan(ctx, dbgen.FindIntakePlanParams{SourceID: source.ID, Params: params, Scope: scope})
	if errors.Is(err, pgx.ErrNoRows) {
		row, saveErr := q.SaveCollectionPlan(ctx, dbgen.SaveCollectionPlanParams{
			ID: uuid.NewV7(), SourceID: source.ID, Capability: plan.Capability, Params: params, Scope: scope,
			Policy: policy, AnchorAt: timestamp(plan.Anchor), IntervalSeconds: int32(plan.Interval / time.Second),
		})
		id, err = row.ID, saveErr
	}
	if err != nil {
		return err
	}
	ok, err := q.AttachCollectionPlan(ctx, dbgen.AttachCollectionPlanParams{PlanID: id, BindingID: binding.ID, SourceID: source.ID})
	if err == nil && !ok {
		err = ErrIntakeState
	}
	return err
}

func publishIntake(ctx context.Context, q *dbgen.Queries, candidate uuid.UUID, game dbgen.Game) error {
	if !game.IntakeManaged || game.PublicationBlocked || game.Status != catalog.Draft || game.PublishedAt.Valid {
		return nil
	}
	if err := q.SaveApprovedGamePresentation(ctx, game.ID); err != nil {
		return err
	}
	ready, err := q.CanPublishGame(ctx, game.ID)
	if err != nil || !ready {
		return err
	}
	published, err := q.ChangeGameStatus(ctx, dbgen.ChangeGameStatusParams{ID: game.ID, NewStatus: catalog.Published})
	if err != nil {
		return err
	}
	decision := catalog.PublicationDecision{Actor: "game-intake", Reason: "Verified furry relevance and SFW evidence retained with approved presentation"}
	if err := decision.Save(ctx, q, "game", game.ID, published.Revision, "publish"); err != nil {
		return err
	}
	return q.MarkIntakePublished(ctx, candidate)
}

func refreshIntakeProfiles(ctx context.Context, q *dbgen.Queries, generation uuid.UUID, source *uuid.UUID, scopeKey string) error {
	rows, err := q.RefreshIntakeProfiles(ctx, dbgen.RefreshIntakeProfilesParams{
		Generation: generation, SourceID: source, ScopeKey: pgtype.Text{String: scopeKey, Valid: scopeKey != ""},
	})
	if err != nil {
		return err
	}
	for _, row := range rows {
		if row.Reason == "merge_suggested" {
			continue
		}
		game, err := q.LockGame(ctx, *row.GameID)
		if err != nil {
			return err
		}
		if err := publishIntake(ctx, q, row.ID, game); err != nil {
			return err
		}
	}
	return nil
}
