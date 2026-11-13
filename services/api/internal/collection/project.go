package collection

import (
	"cmp"
	"context"
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"errors"
	"slices"
	"strconv"
	"strings"
	"time"
	"uuid"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/librecosta/gofurry/services/api/internal/database/dbgen"
)

type result struct {
	observation
	Position  int64
	Confirmed time.Time
	Withdrawn bool
}

func (s *Service) source(ctx context.Context, q *dbgen.Queries, r result, at time.Time) (dbgen.CollectionSource, []byte, []byte, error) {
	if r.Scope.Namespace != s.namespace || !v7(r.TargetID) || !digest.MatchString(r.ScopeKey) || r.Position < 1 || at.IsZero() ||
		!scopePart.MatchString(r.Scope.EgressRegion) ||
		(r.Scope.Locale != "" && !scopePart.MatchString(r.Scope.Locale)) || (r.Scope.Region != "" && !scopePart.MatchString(r.Scope.Region)) {
		return dbgen.CollectionSource{}, nil, nil, errProtocol
	}
	semantic, _ := json.Marshal(r.Scope.scope)
	rr := r.Scope.Dimensions["rrtype"]
	src, err := q.CollectionResultSource(ctx, dbgen.CollectionResultSourceParams{
		TargetID: &r.TargetID, Namespace: s.namespace, Capability: r.Capability, Scope: semantic, EvidenceAt: timestamp(at), Rrtype: rr,
	})
	if err != nil {
		return src, nil, nil, err
	}
	if r.Schema != 1 {
		return src, nil, nil, errSchema
	}
	spec, ok := capabilities[r.Capability]
	if !ok || src.ResourceType != spec.resource || !strings.HasPrefix(r.Capability, src.Provider+".") ||
		!slices.Contains(spec.sections, r.Scope.Dimensions["section"]) || !scopeMatches(r.Scope, r.ScopeKey) ||
		(!r.Withdrawn && r.Source != src.Provider) {
		return src, nil, nil, errProtocol
	}
	if r.Capability == "web.dns" {
		if len(r.Scope.Dimensions) != 3 || !slices.Contains(dnsTypes, rr) || r.Scope.Dimensions["resolver"] != "google-public-dns" {
			return src, nil, nil, errProtocol
		}
	} else if len(r.Scope.Dimensions) != 1 {
		return src, nil, nil, errProtocol
	}
	scopeJSON, _ := json.Marshal(r.Scope, json.Deterministic(true))
	return src, scopeJSON, semantic, nil
}

func validObservation(r result) bool {
	return v7(r.ID) && !r.ObservedAt.IsZero() && !r.ReceivedAt.IsZero() &&
		!r.ObservedAt.After(r.ReceivedAt.Add(2*time.Minute)) && digest.MatchString(r.Hash) &&
		(r.Evidence == "" || evidenceCode.MatchString(r.Evidence)) && r.Sample == (r.Capability == "steam.players") &&
		len(r.Data) <= 32<<10
}

func (s *Service) project(ctx context.Context, q *dbgen.Queries, generation, epoch uuid.UUID, r result, staging bool) error {
	if !r.Withdrawn && r.State != "present" && r.State != "confirmed_empty" {
		return nil
	}
	at := r.ObservedAt
	if staging && !r.Confirmed.IsZero() {
		at = r.Confirmed
	}
	src, scopeJSON, semantic, err := s.source(ctx, q, r, at)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	var c compact
	if r.Withdrawn {
		c = compact{Data: []byte("{}"), Facts: map[string]jsontext.Value{}, Ruleset: projectionRules}
	} else {
		if !validObservation(r) {
			return errProtocol
		}
		if src.Provider == "web" {
			c, err = normalizeWeb(r.observation, src.ExternalKey)
		} else {
			c, err = normalizeSteam(r.observation, src.ExternalKey)
		}
		if err != nil {
			if src.Provider == "steam" && r.Capability == intakeCapability {
				return nil // An invalid Storefront payload cannot replace verified evidence.
			}
			return err
		}
	}
	prior, err := q.CollectionCurrent(ctx, dbgen.CollectionCurrentParams{
		Generation: generation, SourceID: src.ID, Capability: r.Capability, OutputSchema: r.Schema, ScopeKey: r.ScopeKey,
	})
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return err
	}
	upgrade := r.Capability == intakeCapability && c.Ruleset == steamProjectionRules && prior.Ruleset == projectionRules
	if r.Position < prior.Position || (r.Position == prior.Position && (prior.ObservationID == nil || *prior.ObservationID != r.ID ||
		!upgrade && !r.Confirmed.After(prior.ConfirmedAt.Time))) {
		return nil
	}
	var id *uuid.UUID
	if !r.Withdrawn {
		id = &r.ID
	}
	facts, _ := json.Marshal(c.Facts, json.Deterministic(true))
	err = q.PutCollectionCurrent(ctx, dbgen.PutCollectionCurrentParams{
		Generation: generation, SourceID: src.ID, Capability: r.Capability, OutputSchema: r.Schema, ScopeKey: r.ScopeKey, Scope: scopeJSON,
		Position: r.Position, State: r.State, ObservationID: id, ObservedAt: timestamp(r.ObservedAt), ReceivedAt: timestamp(r.ReceivedAt),
		ConfirmedAt: timestamp(r.Confirmed), ContentHash: r.Hash, Evidence: r.Evidence, Ruleset: c.Ruleset, Data: c.Data, Facts: facts,
	})
	if err != nil || staging {
		return err
	}
	if src.Provider == "steam" {
		if r.Capability == intakeCapability && r.Scope.Dimensions["section"] == "details" {
			if err := refreshIntakeProfiles(ctx, q, generation, &src.ID, r.ScopeKey); err != nil {
				return err
			}
		}
		if r.Capability == intakeCapability {
			if err := saveSteamMetadata(ctx, q, generation, epoch, &src.ID, r.ScopeKey); err != nil {
				return err
			}
		}
		if r.Withdrawn || r.Position <= prior.Position {
			return nil
		}
		return gameFacts(ctx, q, src.ID, r, c)
	}
	if r.Position > prior.Position {
		if err := siteFacts(ctx, q, src.ID, epoch, r, scopeJSON, semantic, prior, c); err != nil {
			return err
		}
	}
	if r.Capability == "web.http" || r.Capability == "web.tls" {
		if err := saveSiteTechnology(ctx, q, generation, &src.ID, r.ScopeKey); err != nil {
			return err
		}
	}
	return siteCoverage(ctx, q, src.ID, r, semantic)
}

func saveSiteTechnology(ctx context.Context, q *dbgen.Queries, generation uuid.UUID, source *uuid.UUID, scopeKey string) error {
	return q.SaveSiteTechnology(ctx, dbgen.SaveSiteTechnologyParams{
		Generation: generation, SourceID: source, ScopeKey: pgtype.Text{String: scopeKey, Valid: scopeKey != ""},
	})
}

func siteFacts(ctx context.Context, q *dbgen.Queries, sourceID, epoch uuid.UUID, r result, scopeJSON, semantic []byte,
	prior dbgen.CollectionCurrent, c compact) error {
	bindings, err := q.SiteCollectionBindingsAt(ctx, dbgen.SiteCollectionBindingsAtParams{
		SourceID: sourceID, Capability: r.Capability, Scope: semantic, EvidenceAt: timestamp(r.ObservedAt), Rrtype: r.Scope.Dimensions["rrtype"],
	})
	if err != nil {
		return err
	}
	var previous map[string]jsontext.Value
	if len(prior.Facts) != 0 && json.Unmarshal(prior.Facts, &previous) != nil {
		return errProtocol
	}
	for _, binding := range bindings {
		// A newly associated Site cannot inherit claims from before its own
		// approved interval, even when it shares an existing Target.
		before := previous
		priorRules := prior.Ruleset
		if prior.ObservedAt.Time.Before(binding.AttributableSince.Time) && prior.ConfirmedAt.Time.Before(binding.AttributableSince.Time) {
			before, priorRules = nil, ""
		}
		if r.Withdrawn {
			for key, old := range before {
				if err = addSiteFact(ctx, q, *binding.SiteID, binding.ID, epoch, r, scopeJSON, key, "withdrawn", old, nil, c.Ruleset, priorRules); err != nil {
					return err
				}
			}
			continue
		}
		kind := "change"
		if before == nil {
			kind = "baseline"
		}
		for key, value := range c.Facts {
			old := before[key]
			if len(old) != 0 && sameParams(old, value) {
				continue
			}
			if strings.HasPrefix(key, "tech.detected.") && priorRules != "" && priorRules != c.Ruleset {
				// A detector revision may change recognition. Refresh current evidence
				// and make no historical claim of a technology change.
				continue
			}
			if err = addSiteFact(ctx, q, *binding.SiteID, binding.ID, epoch, r, scopeJSON, key, kind, old, value, c.Ruleset, priorRules); err != nil {
				return err
			}
		}
	}
	return nil
}

func addSiteFact(ctx context.Context, q *dbgen.Queries, site, binding, epoch uuid.UUID, r result, scopeJSON []byte,
	key, kind string, prior, value []byte, ruleset, priorRuleset string) error {
	var id *uuid.UUID
	if !r.Withdrawn {
		id = &r.ID
	}
	return q.AddSiteEvidenceChange(ctx, dbgen.AddSiteEvidenceChangeParams{
		ID: uuid.NewV7(), SiteID: site, BindingID: binding, TargetID: r.TargetID, Epoch: epoch, Position: r.Position, Capability: r.Capability,
		OutputSchema: r.Schema, ScopeKey: r.ScopeKey, Scope: scopeJSON, FactKey: key, EventKind: kind, PriorValue: prior, Value: value, Ruleset: ruleset,
		PriorRuleset: priorRuleset, SourceObservationID: id, ContentHash: r.Hash, Evidence: r.Evidence,
		FirstObservedAt: timestamp(r.ObservedAt), SourceObservedAt: timestamp(r.ObservedAt), ReceivedAt: timestamp(r.ReceivedAt),
	})
}

func siteCoverage(ctx context.Context, q *dbgen.Queries, sourceID uuid.UUID, r result, semantic []byte) error {
	if r.Withdrawn || r.Confirmed.IsZero() {
		return nil
	}
	bindings, err := q.SiteCollectionBindingsAt(ctx, dbgen.SiteCollectionBindingsAtParams{
		SourceID: sourceID, Capability: r.Capability, Scope: semantic, EvidenceAt: timestamp(r.Confirmed), Rrtype: r.Scope.Dimensions["rrtype"],
	})
	if err != nil {
		return err
	}
	for _, binding := range bindings {
		err = q.PutSiteDailyCoverage(ctx, dbgen.PutSiteDailyCoverageParams{
			SiteID: *binding.SiteID, SourceID: sourceID, BindingID: binding.ID, Capability: r.Capability, OutputSchema: r.Schema,
			ScopeKey: r.ScopeKey, ConfirmedAt: timestamp(r.Confirmed), EvidenceState: r.State, ObservationID: r.ID,
		})
		if err != nil {
			return err
		}
	}
	return nil
}

func gameFacts(ctx context.Context, q *dbgen.Queries, sourceID uuid.UUID, r result, c compact) error {
	for _, item := range c.News {
		data, _ := json.Marshal(item)
		if err := q.PutGameEvidenceNews(ctx, dbgen.PutGameEvidenceNewsParams{
			SourceID: sourceID, ScopeKey: r.ScopeKey, Gid: item.GID, PublishedAt: timestamp(item.PublishedAt), ObservedAt: timestamp(r.ObservedAt),
			ObservationID: r.ID, Value: data,
		}); err != nil {
			return err
		}
	}
	if len(c.News) > 0 {
		if err := q.BoundGameEvidenceNews(ctx, dbgen.BoundGameEvidenceNewsParams{SourceID: sourceID, ScopeKey: r.ScopeKey}); err != nil {
			return err
		}
	}
	if c.Players != nil && !r.ObservedAt.Before(time.Now().Add(-30*24*time.Hour)) {
		return q.PutGameEvidencePlayers(ctx, dbgen.PutGameEvidencePlayersParams{
			SourceID: sourceID, ScopeKey: r.ScopeKey, ObservedAt: timestamp(r.ObservedAt), ObservationID: r.ID, Players: *c.Players,
		})
	}
	return nil
}

func applyResult(ev event) (result, error) {
	switch ev.Action {
	case "upsert", "sample":
		if ev.Observation == nil || ev.Observation.identity != ev.identity ||
			(ev.Action == "sample") != ev.Observation.Sample || ev.Committed.IsZero() {
			return result{}, errProtocol
		}
		return result{observation: *ev.Observation, Position: ev.Position, Confirmed: ev.Committed}, nil
	case "withdraw":
		if ev.Observation != nil || ev.Committed.IsZero() {
			return result{}, errProtocol
		}
		return result{observation: observation{identity: ev.identity, Scope: ev.Tombstone, State: "withdrawn",
			ObservedAt: ev.Committed, ReceivedAt: ev.Committed, Evidence: "scope_withdrawn"}, Position: ev.Position, Withdrawn: true}, nil
	default:
		return result{}, errProtocol
	}
}

func (s *Service) applyPage(ctx context.Context, q *dbgen.Queries, epoch, generation uuid.UUID, events []event, staging bool) error {
	slices.SortStableFunc(events, func(a, b event) int {
		return cmp.Compare(a.Position, b.Position)
	})
	for _, ev := range events {
		r, err := applyResult(ev)
		if err == nil {
			err = s.project(ctx, q, generation, epoch, r, staging)
		}
		if err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) sweepPage(ctx context.Context, q *dbgen.Queries, generation, epoch uuid.UUID, rows []currentItem) error {
	for _, item := range rows {
		if item.Source == "steam" && item.Capability == intakeCapability && item.State == "present" {
			if item.Confirmed.IsZero() || item.Confirmed.Before(item.ReceivedAt) {
				return errProtocol
			}
			r := result{observation: item.observation, Position: item.Position, Confirmed: item.Confirmed}
			if err := s.project(ctx, q, generation, epoch, r, true); err != nil {
				return err
			}
			if r.Scope.Dimensions["section"] == "details" {
				if err := refreshIntakeProfiles(ctx, q, generation, nil, r.ScopeKey); err != nil {
					return err
				}
			}
			if err := saveSteamMetadata(ctx, q, generation, epoch, nil, r.ScopeKey); err != nil {
				return err
			}
			continue
		}
		if item.Source != "web" || item.State != "present" && item.State != "confirmed_empty" {
			continue
		}
		if item.Confirmed.IsZero() || item.Confirmed.Before(item.ReceivedAt) {
			return errProtocol
		}
		r := result{observation: item.observation, Position: item.Position, Confirmed: item.Confirmed}
		source, _, semantic, err := s.source(ctx, q, r, r.Confirmed)
		if errors.Is(err, pgx.ErrNoRows) {
			continue
		}
		if err != nil {
			return err
		}
		if !validObservation(r) {
			return errProtocol
		}
		if _, err = normalizeWeb(r.observation, source.ExternalKey); err != nil {
			return err
		}
		n, err := q.ConfirmCollectionCurrent(ctx, dbgen.ConfirmCollectionCurrentParams{
			Generation: generation, SourceID: source.ID, Capability: r.Capability, OutputSchema: r.Schema, ScopeKey: r.ScopeKey,
			Position: r.Position, ObservationID: &r.ID, ConfirmedAt: timestamp(r.Confirmed),
		})
		if err != nil {
			return err
		}
		if n != 0 {
			if err = q.ActivateSiteBaselines(ctx, dbgen.ActivateSiteBaselinesParams{
				Generation: generation, Epoch: epoch, SourceID: &source.ID, ScopeKey: pgtype.Text{String: r.ScopeKey, Valid: true},
			}); err != nil {
				return err
			}
			if r.Capability == "web.http" || r.Capability == "web.tls" {
				if err = saveSiteTechnology(ctx, q, generation, &source.ID, r.ScopeKey); err != nil {
					return err
				}
			}
		}
		if err = siteCoverage(ctx, q, source.ID, r, semantic); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) pageLimit() string { return strconv.Itoa(s.pageSize) }
