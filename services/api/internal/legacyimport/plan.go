package legacyimport

import (
	"context"
	"errors"
	"strings"
	"uuid"

	"github.com/jackc/pgx/v5"
	"github.com/librecosta/gofurry/services/api/internal/catalog"
	"github.com/librecosta/gofurry/services/api/internal/database/dbgen"
)

type targetState struct {
	ID       uuid.UUID `json:"id"`
	PublicID string    `json:"public_id,omitempty"`
	Revision int64     `json:"revision"`
	Preimage string    `json:"preimage_sha256"`
}

type planItem struct {
	Source         key          `json:"source"`
	Intent         string       `json:"intent_sha256"`
	Action         string       `json:"action"`
	Expected       *targetState `json:"expected,omitempty"`
	Issue          string       `json:"issue,omitempty"`
	Hint           bool         `json:"possible_existing_match,omitempty"`
	RelationsIssue string       `json:"relations_issue,omitempty"`
	LogoIssue      string       `json:"logo_issue,omitempty"`
	LifecycleIssue string       `json:"lifecycle_issue,omitempty"`
}

type Plan struct {
	Version   int        `json:"plan_schema_version"`
	Snapshot  string     `json:"snapshot_id"`
	Backup    string     `json:"backup_sha256"`
	Content   string     `json:"content_sha256"`
	Decisions string     `json:"decisions_sha256"`
	Target    string     `json:"target_scope_sha256"`
	Items     []planItem `json:"units"`
}

func (p Plan) Bytes() []byte { return append(encode(p), '\n') }

func planBase(c Config, b blueprint, decisions string) Plan {
	return Plan{BootstrapPlanVersion, c.Snapshot, c.Backup, c.Content, decisions, c.target(), make([]planItem, 0, len(b.Items))}
}

func createPlan(ctx context.Context, c Config, db *pgx.Conn, b blueprint, decisions string) (Plan, error) {
	p := planBase(c, b, decisions)
	tx, err := db.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return p, errors.New("cannot open read-only preflight snapshot")
	}
	defer tx.Rollback(context.Background())
	q := dbgen.New(tx)
	for _, it := range b.Items {
		unit := planItem{Source: it.Key, Intent: digest(encode(it)), RelationsIssue: it.RelationIssue, LogoIssue: it.LogoIssue, LifecycleIssue: it.LifecycleIssue}
		unit.Action, unit.Expected, unit.Issue, err = assess(ctx, q, it)
		if err != nil {
			return p, errors.New("preflight failed while checking target identity")
		}
		if unit.Action == "create" {
			unit.Hint, err = matchHint(ctx, q, it)
			if err != nil {
				return p, errors.New("preflight failed while checking existing content")
			}
		}
		p.Items = append(p.Items, unit)
	}
	if tx.Commit(ctx) != nil {
		return p, errors.New("cannot complete read-only preflight")
	}
	return p, nil
}

func loadPlan(path, expected string, c Config, b blueprint, decisions string) (Plan, error) {
	var p Plan
	if !digestPattern.MatchString(expected) {
		return p, errors.New("apply and verify require the independently confirmed plan SHA256")
	}
	raw, err := readFile(path, maxPlanBytes)
	if err != nil || digest(raw) != expected || decode(raw, &p) != nil {
		return p, errors.New("cannot verify approved plan bytes")
	}
	base := planBase(c, b, decisions)
	if p.Version != base.Version || p.Snapshot != base.Snapshot || p.Backup != base.Backup || p.Content != base.Content ||
		p.Decisions != base.Decisions || p.Target != base.Target || len(p.Items) != len(b.Items) {
		return p, errors.New("plan targets a different input, rule or environment")
	}
	for i, unit := range p.Items {
		it := b.Items[i]
		if unit.Source.String() != it.Key.String() || unit.Intent != digest(encode(it)) {
			return p, errors.New("plan intentions no longer match the sealed input")
		}
	}
	return p, nil
}

func mapping(ctx context.Context, q *dbgen.Queries, k key) (dbgen.FindLegacyImportMappingRow, error) {
	return q.FindLegacyImportMapping(ctx, dbgen.FindLegacyImportMappingParams{LegacyDeployment: deployment, Kind: kind(k.Table), LegacyID: k.first()})
}

func readTarget(ctx context.Context, q *dbgen.Queries, k key, id uuid.UUID) (targetState, error) {
	v, err := q.ReadLegacyEntity(ctx, dbgen.ReadLegacyEntityParams{TargetID: id, Kind: kind(k.Table)})
	if err != nil {
		return targetState{}, err
	}
	if uuidVersion(id) != 7 || (k.Table == sites || k.Table == games || k.Table == collections) && !catalog.ValidPublicID(v.PublicID) {
		return targetState{}, catalog.ErrConflict
	}
	return targetState{id, v.PublicID, v.Revision, digest(v.Snapshot)}, nil
}

func assess(ctx context.Context, q *dbgen.Queries, it intent) (string, *targetState, string, error) {
	m, err := mapping(ctx, q, it.Key)
	switch {
	case err == nil:
		state, err := readTarget(ctx, q, it.Key, m.TargetID)
		if errors.Is(err, pgx.ErrNoRows) || errors.Is(err, catalog.ErrConflict) || err == nil && state.PublicID != m.PublicIDAtLink {
			return "conflict", nil, "mapped_target_integrity", nil
		}
		if err != nil {
			return "", nil, "", err
		}
		if it.Link != nil && (it.Link.ID != state.ID || it.Link.PublicID != state.PublicID) {
			return "conflict", &state, "mapping_cannot_change_target", nil
		}
		return "reuse", &state, "", nil
	case !errors.Is(err, pgx.ErrNoRows):
		return "", nil, "", err
	}

	if it.Link != nil {
		state, err := readTarget(ctx, q, it.Key, it.Link.ID)
		if errors.Is(err, pgx.ErrNoRows) || errors.Is(err, catalog.ErrConflict) {
			return "conflict", nil, "adopted_target_missing", nil
		}
		if err != nil {
			return "", nil, "", err
		}
		if state.Revision != it.Link.Revision || state.PublicID != it.Link.PublicID {
			return "conflict", &state, "adopted_target_changed", nil
		}
		n, err := q.LegacyImportTargetOwners(ctx, dbgen.LegacyImportTargetOwnersParams{LegacyDeployment: deployment, Kind: kind(it.Key.Table), TargetID: state.ID})
		if err != nil {
			return "", nil, "", err
		}
		if !it.Alias && n != 0 {
			return "conflict", &state, "alias_requires_explicit_decision", nil
		}
		if issue, err := checkSources(ctx, q, it, &state); err != nil || issue != "" {
			return "conflict", &state, issue, err
		}
		return "adopt", &state, "", nil
	}

	if it.IdentityIssue != "" {
		action := "defer"
		if strings.Contains(it.IdentityIssue, "conflict") || strings.Contains(it.IdentityIssue, "source") || strings.Contains(it.IdentityIssue, "identity") {
			action = "conflict"
		}
		return action, nil, it.IdentityIssue, nil
	}
	if it.Key.Table == tags || it.Key.Table == categories {
		taken, err := q.LegacyEditorialCodeExists(ctx, dbgen.LegacyEditorialCodeExistsParams{Kind: kind(it.Key.Table), Code: it.Code})
		if err != nil {
			return "", nil, "", err
		}
		if taken {
			return "conflict", nil, "target_tag_code_taken", nil
		}
	}
	if issue, err := checkSources(ctx, q, it, nil); err != nil || issue != "" {
		return "conflict", nil, issue, err
	}
	return "create", nil, "", nil
}

func checkSources(ctx context.Context, q *dbgen.Queries, it intent, desired *targetState) (string, error) {
	for _, s := range it.Sources {
		owners, err := q.LegacySourceOwners(ctx, dbgen.LegacySourceOwnersParams{PlatformCode: s.PlatformCode, ExternalKey: s.ExternalKey})
		if err != nil {
			return "", err
		}
		if len(owners) == 0 && desired == nil {
			continue
		}
		if desired == nil || len(owners) != 1 || owners[0].GameID != desired.ID || !owners[0].Active ||
			owners[0].ExternalKey != s.ExternalKey || owners[0].SourceUrl != s.SourceURL {
			return "game_source_owner_conflict", nil
		}
	}
	return "", nil
}

func matchHint(ctx context.Context, q *dbgen.Queries, it intent) (bool, error) {
	var names, urls []string
	for _, l := range it.Editorial.Localizations {
		names = append(names, strings.ToLower(l.Name))
	}
	switch it.Key.Table {
	case sites:
		for _, a := range it.Addresses {
			urls = append(urls, a.URL)
		}
		if it.SiteHintURL != "" {
			urls = append(urls, it.SiteHintURL)
		}
		return q.LegacySiteHint(ctx, dbgen.LegacySiteHintParams{Names: names, Urls: urls, Slug: it.Editorial.Slug})
	case games:
		return q.LegacyGameHint(ctx, dbgen.LegacyGameHintParams{Names: names, Slug: it.Editorial.Slug})
	default:
		return false, nil
	}
}

func phaseIntent(it intent, phase string) string {
	var data any
	switch phase {
	case "identity":
		data = []any{it.Key, it.Editorial, it.Taxonomy, it.Addresses, it.Sources, it.Code, it.Sort, it.Category, it.ArchivedAt, it.Adult, it.Link, it.Alias}
	case "relations":
		data = it.Relations
	case "logo":
		data = it.Logo
	case "lifecycle":
		data = []any{it.Archive, it.Publish, it.PublishProof}
	}
	return digest(encode(data))
}

func samePreimage(current, expected *targetState) bool {
	return current != nil && expected != nil && current.ID == expected.ID && current.PublicID == expected.PublicID &&
		current.Revision >= expected.Revision && current.Preimage == expected.Preimage
}
