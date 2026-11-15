package legacyimport

import (
	"bytes"
	"context"
	"errors"
	"slices"
	"time"
	"uuid"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/librecosta/gofurry/services/api/internal/catalog"
	"github.com/librecosta/gofurry/services/api/internal/database/dbgen"
	"github.com/librecosta/gofurry/services/api/internal/legacyseed"
	"github.com/librecosta/gofurry/services/api/internal/storage"
)

type Options struct {
	Mode     string
	PlanFile string
	PlanSHA  string
}

type phaseResult struct {
	Source       key          `json:"source"`
	Phase        string       `json:"phase"`
	Result       string       `json:"result"`
	Issue        string       `json:"issue,omitempty"`
	Retryable    bool         `json:"retryable,omitempty"`
	Target       *targetState `json:"target,omitempty"`
	DecisionRefs []string     `json:"decision_refs,omitempty"`
}

type collectionResult struct {
	Source key `json:"source"`
	dbgen.LegacyCollectionCountsRow
}

type Report struct {
	Status       string                    `json:"status"`
	PlanSHA      string                    `json:"plan_sha256,omitempty"`
	Content      string                    `json:"content_sha256"`
	Target       string                    `json:"target_scope_sha256"`
	SourceRows   int                       `json:"source_rows"`
	Counts       map[string]int            `json:"counts"`
	SourceTables map[string]map[string]int `json:"source_tables,omitempty"`
	PhaseCounts  map[string]int            `json:"phase_counts,omitempty"`
	Phases       []phaseResult             `json:"phases,omitempty"`
	Rows         []sourceResult            `json:"rows,omitempty"`
	Collections  []collectionResult        `json:"collections,omitempty"`
	Assets       map[string]int            `json:"site_logos,omitempty"`
}

type engine struct {
	ctx                context.Context
	conn               *pgx.Conn
	service            *catalog.Service
	bundle             *legacyseed.Bundle
	store              *storage.Store
	verified           map[string]bool
	report             *Report
	planSHA, decisions string
}

func Run(ctx context.Context, c Config, opts Options) (Plan, Report, error) {
	var plan Plan
	report := Report{Status: "failed", Content: c.Content, Target: c.target(), Counts: map[string]int{}, Assets: map[string]int{}}
	if err := c.validate(); err != nil {
		return plan, report, err
	}
	if opts.Mode != "plan" && opts.Mode != "apply" && opts.Mode != "verify" {
		return plan, report, errors.New("unsupported importer mode")
	}
	bundle, err := legacyseed.OpenBundle(ctx, c.SeedDir, c.Snapshot, c.Backup, c.Content)
	if err != nil {
		return plan, report, err
	}
	defer bundle.Close()
	ds, decisionSHA, err := loadDecisions(c)
	if err != nil {
		return plan, report, err
	}
	b, err := build(ctx, bundle, ds)
	if err != nil {
		return plan, report, err
	}
	if opts.Mode != "plan" {
		plan, err = loadPlan(opts.PlanFile, opts.PlanSHA, c, b, decisionSHA)
		if err != nil {
			return plan, report, err
		}
		report.PlanSHA = opts.PlanSHA
	}
	pool, err := c.open(ctx)
	if err != nil {
		return plan, report, err
	}
	defer pool.Close()
	session, err := pool.Acquire(ctx)
	if err != nil {
		return plan, report, errors.New("cannot connect to isolated PostgreSQL target")
	}
	defer session.Release()
	conn := session.Conn()
	if err = c.guard(ctx, conn); err != nil {
		return plan, report, err
	}
	if opts.Mode == "plan" {
		plan, err = createPlan(ctx, c, conn, b, decisionSHA)
		if err != nil {
			return plan, report, err
		}
		report.PlanSHA = digest(plan.Bytes())
		report.Status = "planned"
		for i, p := range plan.Items {
			it := b.Items[i]
			r := p.Action
			if p.Hint {
				b.issue(it.Key, "possible_existing_match")
			}
			report.Phases = append(report.Phases, phaseResult{Source: p.Source, Phase: "identity", Result: r, Issue: p.Issue, Target: p.Expected, DecisionRefs: it.DecisionRefs})
			if p.RelationsIssue != "" {
				b.issue(it.Key, p.RelationsIssue)
			}
			if p.LogoIssue != "" {
				b.issue(it.Key, p.LogoIssue)
			}
			if p.LifecycleIssue != "" {
				b.issue(it.Key, p.LifecycleIssue)
			}
		}
		report.collect(b)
		return plan, report, nil
	}

	objects, err := c.objects()
	if err != nil {
		return plan, report, err
	}
	if objects != nil {
		defer objects.Close()
	}
	// A shared verification lock excludes simultaneous import writers. Domain
	// transactions use their own row locks and revision guards as before.
	lockQuery := "SELECT pg_try_advisory_lock(1280760113,1)"
	unlockQuery := "SELECT pg_advisory_unlock(1280760113,1)"
	if opts.Mode == "verify" {
		lockQuery = "SELECT pg_try_advisory_lock_shared(1280760113,1)"
		unlockQuery = "SELECT pg_advisory_unlock_shared(1280760113,1)"
	}
	var acquired bool
	if conn.QueryRow(ctx, lockQuery).Scan(&acquired) != nil || !acquired {
		return plan, report, errors.New("another legacy import holds the target lock")
	}
	defer func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		if _, err := conn.Exec(cleanup, unlockQuery); err != nil {
			_ = conn.Close(cleanup)
		}
	}()
	e := engine{ctx: ctx, conn: conn, service: catalog.New(pool), bundle: bundle, store: objects, verified: map[string]bool{},
		report: &report, planSHA: report.PlanSHA, decisions: decisionSHA}
	if opts.Mode == "apply" {
		q := dbgen.New(conn)
		if err = q.RegisterLegacyImportBatch(ctx, dbgen.RegisterLegacyImportBatchParams{PlanSha256: e.planSHA, LegacyDeployment: deployment,
			BackupSha256: c.Backup, ContentSha256: c.Content, DecisionsSha256: decisionSHA}); err != nil {
			return plan, report, errors.New("cannot register import batch")
		}
	}
	e.execute(&b, plan, opts.Mode == "apply")
	report.collect(b)
	report.Assets["unique_objects_verified"] = len(e.verified)
	report.Status = "complete"
	for _, r := range []string{"conflict", "deferred", "retry_pending"} {
		if report.Counts[r] != 0 {
			report.Status = "incomplete"
			return plan, report, ErrIncomplete
		}
	}
	return plan, report, nil
}

func (r *Report) collect(b blueprint) {
	r.Rows = make([]sourceResult, 0, len(b.Rows))
	r.Counts = make(map[string]int)
	r.SourceTables = make(map[string]map[string]int)
	for _, row := range b.Rows {
		r.Rows = append(r.Rows, row)
		r.Counts[row.Disposition]++
		if r.SourceTables[row.Source.Table] == nil {
			r.SourceTables[row.Source.Table] = make(map[string]int)
		}
		r.SourceTables[row.Source.Table][row.Disposition]++
	}
	r.PhaseCounts = make(map[string]int)
	for _, phase := range r.Phases {
		r.PhaseCounts[phase.Result]++
	}
	slices.SortFunc(r.Rows, func(a, b sourceResult) int {
		if a.Source.Table != b.Source.Table {
			if a.Source.Table < b.Source.Table {
				return -1
			}
			return 1
		}
		return slices.Compare(a.Source.IDs, b.Source.IDs)
	})
	r.SourceRows = len(r.Rows)
}

func children(it intent, phase string) []key {
	switch phase {
	case "identity":
		return append(slices.Clone(it.AddressRows), it.GroupRows...)
	case "relations":
		result := make([]key, 0, len(it.Relations))
		for _, rel := range it.Relations {
			result = append(result, rel.Key)
		}
		return result
	}
	return nil
}

func severity(disposition string) int {
	switch disposition {
	case "conflict":
		return 5
	case "retry_pending":
		return 4
	case "deferred":
		return 3
	case "applied":
		return 2
	case "verified_existing":
		return 1
	default:
		return 0
	}
}

func (e *engine) record(b *blueprint, it intent, phase, outcome, issue string, state *targetState) {
	status := outcome
	switch outcome {
	case "created", "linked", "alias", "applied":
		status = "applied"
	case "reused", "verified_existing":
		status = "verified_existing"
	}
	e.report.Phases = append(e.report.Phases, phaseResult{Source: it.Key, Phase: phase, Result: outcome, Issue: issue,
		Retryable: status == "retry_pending", Target: state, DecisionRefs: it.DecisionRefs})
	for _, k := range append([]key{it.Key}, children(it, phase)...) {
		if severity(status) > severity(b.Rows[k.String()].Disposition) {
			b.set(k, status)
		}
		if issue != "" {
			b.issue(k, issue)
		}
	}
}

func (e *engine) execute(b *blueprint, p Plan, apply bool) {
	usable := make([]bool, len(b.Items))
	related := make([]bool, len(b.Items))
	owned := make([]bool, len(b.Items))

	for _, i := range identityOrder(b.Items) {
		it := b.Items[i]
		if p.Items[i].Hint {
			b.issue(it.Key, "possible_existing_match")
		}
		outcome, issue, state := e.step(it, p.Items[i], "identity", apply)
		e.record(b, it, "identity", outcome, issue, state)
		usable[i] = issue == ""
		if usable[i] {
			m, err := mapping(e.ctx, dbgen.New(e.conn), it.Key)
			if err != nil {
				e.record(b, it, "identity", "retry_pending", "mapping_read_failed", state)
				usable[i] = false
			} else {
				owned[i] = m.Origin == "created"
			}
		}
	}

	for i, it := range b.Items {
		if !usable[i] || !owned[i] {
			continue
		}
		if it.Key.Table != games && it.Key.Table != collections {
			continue
		}
		if it.RelationIssue != "" {
			e.record(b, it, "relations", "deferred", it.RelationIssue, nil)
			continue
		}
		if len(it.Relations) == 0 {
			related[i] = e.checkOmittedPhase(b, it, "relations")
			continue
		}
		outcome, issue, state := e.step(it, p.Items[i], "relations", apply)
		related[i] = issue == ""
		e.record(b, it, "relations", outcome, issue, state)
	}
	for i, it := range b.Items {
		if !usable[i] || !owned[i] {
			continue
		}
		if it.LogoIssue != "" {
			e.report.Assets["deferred"]++
			e.record(b, it, "logo", "deferred", it.LogoIssue, nil)
		}
		if it.Logo != "" {
			e.report.Assets["approved_references"]++
			outcome, issue, state := e.step(it, p.Items[i], "logo", apply)
			if issue == "" {
				e.report.Assets["verified_references"]++
			} else {
				e.report.Assets["pending_references"]++
			}
			e.record(b, it, "logo", outcome, issue, state)
		} else if it.Key.Table == sites {
			e.checkOmittedPhase(b, it, "logo")
		}
		if it.Publish && it.Key.Table == collections && !related[i] {
			e.record(b, it, "lifecycle", "deferred", "collection_relations_incomplete", nil)
			continue
		}
		if it.LifecycleIssue != "" {
			e.record(b, it, "lifecycle", "deferred", it.LifecycleIssue, nil)
		}
		if it.Archive || it.Publish {
			outcome, issue, state := e.step(it, p.Items[i], "lifecycle", apply)
			e.record(b, it, "lifecycle", outcome, issue, state)
		} else if it.Key.Table == sites || it.Key.Table == games || it.Key.Table == collections {
			e.checkOmittedPhase(b, it, "lifecycle")
		}
		if it.Key.Table == collections {
			m, err := mapping(e.ctx, dbgen.New(e.conn), it.Key)
			if err == nil {
				counts, err := dbgen.New(e.conn).LegacyCollectionCounts(e.ctx, m.TargetID)
				if err == nil {
					e.report.Collections = append(e.report.Collections, collectionResult{it.Key, counts})
					continue
				}
			}
			e.record(b, it, "relations", "retry_pending", "collection_reconciliation_failed", nil)
		}
	}
	// Also reconcile currently adopted collections, which were never edited.
	for i, it := range b.Items {
		if !usable[i] || owned[i] || it.Key.Table != collections {
			continue
		}
		q := dbgen.New(e.conn)
		m, err := mapping(e.ctx, q, it.Key)
		if err == nil {
			counts, err := q.LegacyCollectionCounts(e.ctx, m.TargetID)
			if err == nil {
				e.report.Collections = append(e.report.Collections, collectionResult{it.Key, counts})
				continue
			}
		}
		e.record(b, it, "relations", "retry_pending", "collection_reconciliation_failed", nil)
	}
}

func (e *engine) checkOmittedPhase(b *blueprint, it intent, phase string) bool {
	q := dbgen.New(e.conn)
	row, err := mapping(e.ctx, q, it.Key)
	if err == nil {
		_, err = q.PriorLegacyImportPhase(e.ctx, dbgen.PriorLegacyImportPhaseParams{
			SourceKey: it.Key.String(), TargetID: row.TargetID, Phase: phase})
		if errors.Is(err, pgx.ErrNoRows) {
			return true
		}
		if err == nil {
			e.record(b, it, phase, "conflict", "completed_phase_omitted", nil)
			return false
		}
	}
	e.record(b, it, phase, "retry_pending", "prior_phase_unavailable", nil)
	return false
}

func identityOrder(items []intent) []int {
	order := make([]int, 0, len(items))
	for i, it := range items {
		if !it.Alias {
			order = append(order, i)
		}
	}
	for i, it := range items {
		if it.Alias {
			order = append(order, i)
		}
	}
	return order
}

type unitError struct{ outcome, code string }

func (e unitError) Error() string { return e.code }
func conflict(code string) error  { return unitError{"conflict", code} }
func deferred(code string) error  { return unitError{"deferred", code} }
func retry(code string) error     { return unitError{"retry_pending", code} }

func outcome(err error) (string, string) {
	var failure unitError
	var sqlErr *pgconn.PgError
	switch {
	case errors.As(err, &failure):
		return failure.outcome, failure.code
	case errors.Is(err, catalog.ErrInvalid) || errors.Is(err, catalog.ErrConflict) || errors.Is(err, catalog.ErrNotFound):
		return "conflict", "catalog_edit_rejected"
	case errors.As(err, &sqlErr) && sqlErr.Code == "23505":
		return "conflict", "unique_target_conflict"
	default:
		return "retry_pending", "transaction_failed"
	}
}

func (e *engine) step(it intent, p planItem, phase string, apply bool) (string, string, *targetState) {
	intentSHA := phaseIntent(it, phase)
	// Do not touch object storage until the mapped, unedited Site is eligible.
	// A second check inside the binding transaction protects changes made
	// during the object request.
	if phase == "logo" {
		if result, issue, state := e.checkLogo(it, intentSHA, apply); result != "" {
			return result, issue, state
		}
	}
	options := pgx.TxOptions{}
	if !apply {
		options.IsoLevel = pgx.RepeatableRead
		options.AccessMode = pgx.ReadOnly
	}
	tx, err := e.conn.BeginTx(e.ctx, options)
	if err != nil {
		return "retry_pending", "transaction_unavailable", nil
	}
	defer tx.Rollback(context.Background())
	q := dbgen.New(tx)

	result, state, err := e.process(q, it, p, phase, intentSHA, apply)
	if err != nil {
		r, code := outcome(err)
		return r, code, state
	}
	if err = tx.Commit(e.ctx); err != nil {
		// A lost COMMIT response is never treated as a fresh write. Reconnect
		// through the next invocation and check the durable receipt first.
		return "retry_pending", "commit_outcome_unknown", state
	}
	return result, "", state
}

func (e *engine) mapped(q *dbgen.Queries, it intent, lock bool) (dbgen.FindLegacyImportMappingRow, *targetState, error) {
	m, err := mapping(e.ctx, q, it.Key)
	if err != nil {
		return m, nil, err
	}
	if lock {
		if err = lockTarget(e.ctx, q, it.Key, m.TargetID); err != nil {
			return m, nil, err
		}
	}
	state, err := readTarget(e.ctx, q, it.Key, m.TargetID)
	if err != nil {
		return m, nil, err
	}
	if state.PublicID != m.PublicIDAtLink {
		return m, &state, conflict("mapped_target_integrity")
	}
	return m, &state, nil
}

func (e *engine) checkLatest(q *dbgen.Queries, it intent, state *targetState) (dbgen.LegacyImportReceipt, error) {
	last, err := q.LatestLegacyImportReceipt(e.ctx, dbgen.LatestLegacyImportReceiptParams{SourceKey: it.Key.String(), TargetID: state.ID})
	if errors.Is(err, pgx.ErrNoRows) {
		return last, conflict("mapped_target_receipt_missing")
	}
	if err != nil {
		return last, err
	}
	if last.PublicID != state.PublicID || last.Revision > state.Revision || last.PostSha256 != state.Preimage {
		return last, conflict("editorial_conflict")
	}
	return last, nil
}

func (e *engine) process(q *dbgen.Queries, it intent, p planItem, phase, intentSHA string, apply bool) (string, *targetState, error) {
	receipt, err := q.ReadLegacyImportReceipt(e.ctx, dbgen.ReadLegacyImportReceiptParams{PlanSha256: e.planSHA, SourceKey: it.Key.String(), Phase: phase})
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return "", nil, err
	}
	committed := err == nil
	m, state, err := e.mapped(q, it, apply)
	if committed {
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				err = conflict("mapped_target_integrity")
			}
			return "", state, err
		}
		if receipt.IntentSha256 != intentSHA || receipt.TargetID != state.ID || receipt.PublicID != state.PublicID || receipt.DecisionSha256 != e.decisions {
			return "", state, conflict("committed_intent_conflict")
		}

		if _, err = e.checkLatest(q, it, state); err != nil {
			return "", state, err
		}
		if phase == "identity" {
			if issue, err := checkSources(e.ctx, q, it, state); err != nil {
				return "", state, err
			} else if issue != "" {
				return "", state, conflict(issue)
			}
		}
		return "reused", state, nil
	}
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return "", state, err
	}
	if errors.Is(err, pgx.ErrNoRows) && m.TargetID != (uuid.UUID{}) {
		return "", state, conflict("mapped_target_integrity")
	}

	if phase == "identity" {
		if p.Action == "conflict" {
			return "", state, conflict(p.Issue)
		}
		if p.Action == "defer" {
			return "", state, deferred(p.Issue)
		}
		if state != nil {
			if p.Action != "reuse" || !samePreimage(state, p.Expected) {
				return "", state, conflict("target_changed_after_plan")
			}
			if _, err = e.checkLatest(q, it, state); err != nil {
				return "", state, err
			}
			// Mappings adopted under reviewed decisions are always map-only.
			// An updated snapshot can only compare the original creation.
			original, err := q.PriorLegacyImportPhase(e.ctx, dbgen.PriorLegacyImportPhaseParams{SourceKey: it.Key.String(), TargetID: state.ID, Phase: phase})
			if err != nil {
				return "", state, err
			}
			if m.Origin == "created" && original.IntentSha256 != intentSHA {
				return "", state, conflict("completed_identity_intent_changed")
			}
			if apply {
				return "verified_existing", state, e.receipt(q, it, phase, intentSHA, "verified_existing", *state)
			}
			return "", state, retry("plan_receipt_missing")
		}
		if !apply {
			return "", nil, retry("plan_receipt_missing")
		}
		return e.identity(q, it, p, intentSHA)
	}
	if state == nil || m.Origin != "created" {
		return "", state, conflict("phase_target_unavailable")
	}
	if _, err = e.checkLatest(q, it, state); err != nil {
		return "", state, err
	}
	prior, err := q.PriorLegacyImportPhase(e.ctx, dbgen.PriorLegacyImportPhaseParams{SourceKey: it.Key.String(), TargetID: state.ID, Phase: phase})
	if err == nil {
		if prior.IntentSha256 != intentSHA {
			return "", state, conflict("completed_phase_intent_changed")
		}
		if apply {
			return "verified_existing", state, e.receipt(q, it, phase, intentSHA, "verified_existing", *state)
		}
		return "", state, retry("plan_receipt_missing")
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return "", state, err
	}
	if !apply {
		return "", state, retry("plan_receipt_missing")
	}
	bound := e.service.InTransaction(q)
	switch phase {
	case "relations":
		err = e.relations(q, bound, it, *state)
	case "logo":
		_, err = bound.SetSiteAsset(e.ctx, state.PublicID, state.Revision, storage.SiteLogo, logoKey(it.Logo))
	case "lifecycle":
		err = e.lifecycle(bound, it, *state)
	}
	if err != nil {
		return "", state, err
	}
	next, err := readTarget(e.ctx, q, it.Key, state.ID)
	if err != nil {
		return "", state, err
	}
	return "applied", &next, e.receipt(q, it, phase, intentSHA, "applied", next)
}

func (e *engine) receipt(q *dbgen.Queries, it intent, phase, intentSHA, result string, state targetState) error {
	return q.AddLegacyImportReceipt(e.ctx, dbgen.AddLegacyImportReceiptParams{PlanSha256: e.planSHA, SourceKey: it.Key.String(),
		Phase: phase, IntentSha256: intentSHA, DecisionSha256: e.decisions, Outcome: result, TargetID: state.ID, PublicID: state.PublicID,
		Revision: state.Revision, PostSha256: state.Preimage})
}

func logoKey(sha string) string { return "sites/logos/" + sha + ".webp" }

func (e *engine) checkLogo(it intent, intentSHA string, apply bool) (string, string, *targetState) {
	q := dbgen.New(e.conn)
	m, state, err := e.mapped(q, it, false)
	if err != nil {
		r, code := outcome(err)
		return r, code, state
	}
	if _, err = e.checkLatest(q, it, state); err != nil {
		r, code := outcome(err)
		return r, code, state
	}
	previous, err := q.PriorLegacyImportPhase(e.ctx, dbgen.PriorLegacyImportPhaseParams{SourceKey: it.Key.String(), TargetID: m.TargetID, Phase: "logo"})
	hasPrevious := err == nil
	if !hasPrevious && !errors.Is(err, pgx.ErrNoRows) {
		r, code := outcome(err)
		return r, code, state
	}
	if hasPrevious && previous.IntentSha256 != intentSHA {
		return "conflict", "completed_phase_intent_changed", state
	}
	if !hasPrevious {
		if !apply {
			return "retry_pending", "plan_receipt_missing", state
		}
		if m.Origin != "created" {
			return "conflict", "phase_target_unavailable", state
		}
		row, err := q.FindYearbookSite(e.ctx, state.PublicID)
		if err != nil {
			return "retry_pending", "site_logo_state_unavailable", state
		}
		if row.Status == catalog.Archived {
			return "deferred", "archived_site_logo_pending", state
		}
		if row.LogoAssetKey.Valid {
			return "conflict", "site_logo_already_bound", state
		}
	}
	if e.store == nil {
		return "retry_pending", "isolated_asset_store_unconfigured", state
	}
	if e.verified[it.Logo] {
		return "", "", state
	}
	asset, err := e.store.VerifySiteAsset(e.ctx, storage.SiteLogo, logoKey(it.Logo))
	if err == nil && asset.SHA256 == it.Logo {
		e.verified[it.Logo] = true
		e.report.Assets["reused_objects"]++
		return "", "", state
	}
	if hasPrevious || !apply || !errors.Is(err, storage.ErrNotFound) {
		return "retry_pending", "site_logo_object_verification_failed", state
	}
	raw, err := e.bundle.LogoBytes(it.Logo)
	if err != nil {
		return "retry_pending", "site_logo_seed_read_failed", state
	}
	asset, err = e.store.ImportSiteAsset(e.ctx, storage.SiteLogo, bytes.NewReader(raw), false)
	if err != nil || asset.SHA256 != it.Logo {
		return "retry_pending", "site_logo_upload_or_delivery_failed", state
	}
	e.verified[it.Logo] = true
	e.report.Assets["uploaded_objects"]++
	return "", "", state
}
