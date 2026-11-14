package collection

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"errors"
	"net/http"
	"regexp"
	"slices"
	"strconv"
	"time"
	"uuid"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/librecosta/gofurry/services/api/internal/database/dbgen"
	"github.com/librecosta/gofurry/services/api/internal/platform"
)

const catalogCapability = "steam.catalog"
const maxDiscoveryPending = 512

type Discovery struct {
	pool   *pgxpool.Pool
	remote *client
	intake *Intake
	config platform.DiscoveryConfig
	region string
}

func NewDiscovery(pool *pgxpool.Pool, cfg *platform.HarvestConfig, intake *Intake) *Discovery {
	if cfg == nil || cfg.Discovery == nil {
		return nil
	}
	d := *cfg.Discovery
	return &Discovery{pool, newClient(platform.HarvestConfig{URL: cfg.URL, ReaderToken: d.ReaderToken, WriterToken: d.WriterToken}), intake, d, cfg.IntakeEgressRegion}
}

func (d *Discovery) Close() { d.remote.http.CloseIdleConnections() }

func (d *Discovery) Run(ctx context.Context, report func()) {
	timer := time.NewTimer(0)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
		}
		if err := d.Tick(ctx); err != nil && ctx.Err() == nil {
			report()
		}
		timer.Reset(d.config.PollInterval)
	}
}

type DiscoveryStatus struct {
	Paused        bool   `json:"paused"`
	Backfill      bool   `json:"backfill"`
	Watermark     int64  `json:"watermark"`
	WindowStarted int64  `json:"window_started"`
	ModifiedSince int64  `json:"modified_since"`
	AppID         int64  `json:"last_appid"`
	Pending       int    `json:"pending"`
	Pages         int64  `json:"pages"`
	Rows          int64  `json:"rows_seen"`
	Windows       int64  `json:"completed_windows"`
	Admitted      int64  `json:"submitted"`
	JobsToday     int32  `json:"jobs_today"`
	SelectedToday int32  `json:"selected_today"`
	ExploredToday int32  `json:"explored_today"`
	LastCode      string `json:"last_code"`
}

func (d *Discovery) Status(ctx context.Context) (DiscoveryStatus, error) {
	q := dbgen.New(d.pool)
	s, err := q.GetSteamDiscovery(ctx, d.config.Namespace)
	if errors.Is(err, pgx.ErrNoRows) {
		return DiscoveryStatus{}, nil
	}
	if err != nil {
		return DiscoveryStatus{}, err
	}
	pending, err := discoveryPending(s)
	return DiscoveryStatus{s.Paused, s.Backfill, s.Watermark, s.WindowStarted, s.ModifiedSince, s.LastAppid, len(pending),
		s.Pages, s.RowsSeen, s.Windows, s.Admitted, s.JobsToday, s.SelectedToday, s.ExploredToday, s.LastCode}, err
}

func (d *Discovery) locked(ctx context.Context, f func(*pgxpool.Conn, *dbgen.Queries, dbgen.SteamDiscoveryState) error) error {
	conn, err := d.pool.Acquire(ctx)
	if err != nil {
		return err
	}
	defer conn.Release()
	q := dbgen.New(conn)
	locked, err := q.LockSteamDiscovery(ctx)
	if err != nil {
		return err
	}
	if !locked {
		return errBusy
	}
	defer func() {
		unlock, cancel := context.WithTimeout(context.WithoutCancel(ctx), time.Second)
		defer cancel()
		if q.UnlockSteamDiscovery(unlock) != nil {
			_ = conn.Conn().Close(unlock)
		}
	}()
	if err = q.InitSteamDiscovery(ctx, d.config.Namespace); err != nil {
		return err
	}
	state, err := q.GetSteamDiscovery(ctx, d.config.Namespace)
	if err != nil {
		return err
	}
	return f(conn, q, state)
}

func (d *Discovery) Configure(ctx context.Context, paused, backfill bool) error {
	return d.locked(ctx, func(_ *pgxpool.Conn, q *dbgen.Queries, s dbgen.SteamDiscoveryState) error {
		if backfill && s.WindowStarted != 0 {
			return errors.New("finish the current scan window before starting a backfill")
		}
		return q.ConfigureSteamDiscovery(ctx, dbgen.ConfigureSteamDiscoveryParams{Namespace: s.Namespace, Paused: paused, StartBackfill: backfill})
	})
}

type discoveryParams struct {
	Since uint32 `json:"if_modified_since"`
	After uint32 `json:"last_appid"`
	Limit uint32 `json:"max_results"`
}
type catalogRow struct {
	ID          uint32 `json:"appid"`
	Name        string `json:"name"`
	Modified    int64  `json:"last_modified"`
	PriceChange uint64 `json:"price_change_number"`
}
type discoveryPage struct {
	Request discoveryParams `json:"request"`
	Apps    []catalogRow    `json:"apps"`
	More    bool            `json:"have_more_results"`
	Last    uint32          `json:"last_appid"`
}
type discoveryPick struct {
	AppID    uint32 `json:"app_id"`
	Modified int64  `json:"last_modified"`
	Priority int    `json:"priority"`
}

func discoveryPending(s dbgen.SteamDiscoveryState) ([]discoveryPick, error) {
	var out []discoveryPick
	if json.Unmarshal(s.Pending, &out) != nil || out == nil || len(out) > maxDiscoveryPending {
		return nil, errProtocol
	}
	return out, nil
}

func (d *Discovery) Tick(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 32*time.Second)
	defer cancel()
	err := d.locked(ctx, func(conn *pgxpool.Conn, q *dbgen.Queries, s dbgen.SteamDiscoveryState) error {
		if s.Paused {
			return nil
		}
		pending, err := discoveryPending(s)
		if err != nil {
			return err
		}
		if len(pending) != 0 {
			pending, err = d.drain(ctx, conn, s.Namespace, pending)
			if err != nil {
				return err
			}
		}
		now := time.Now().UTC()
		if s.NextAt.Valid && now.Before(s.NextAt.Time) || len(pending) > maxDiscoveryPending-max(int(d.config.PageSize), int(s.PageLimit)) {
			return nil
		}
		// Reserve pool capacity for Intake and HTTP. Each tick performs one remote
		// job step, so an outage does not hold an open SQL transaction.
		day := now.Unix() / 86400
		if s.IntentID == nil && s.BudgetDay == day && s.JobsToday >= d.config.JobsPerDay {
			return nil
		}
		if s.WindowStarted == 0 {
			since := max(s.Watermark-3600, 0)
			if s.Watermark == 0 {
				since = now.Add(-48 * time.Hour).Unix()
			}
			if s.Backfill {
				since = 0
			}
			return q.OpenSteamDiscoveryWindow(ctx, dbgen.OpenSteamDiscoveryWindowParams{Namespace: s.Namespace,
				WindowStarted: now.Unix(), ModifiedSince: since, EgressRegion: d.region, PageLimit: d.config.PageSize})
		}
		if s.TargetID == nil {
			return d.ensureTarget(ctx, q, s.Namespace)
		}
		writer := sha256.Sum256([]byte(d.remote.base + "\x00" + d.remote.writer))
		binding := hex.EncodeToString(writer[:])
		if s.IntentID == nil {
			id := uuid.NewV7()
			return q.SaveSteamDiscoveryIntent(ctx, dbgen.SaveSteamDiscoveryIntentParams{Namespace: s.Namespace, IntentID: &id, WriterBinding: binding, BudgetDay: day})
		}
		p := discoveryParams{uint32(s.ModifiedSince), uint32(s.LastAppid), uint32(s.PageLimit)}
		raw, _ := json.Marshal(p)
		req := request{*s.TargetID, catalogCapability, 1, raw, scope{EgressRegion: s.EgressRegion}}
		if s.JobID == nil {
			if s.WriterBinding != binding || !s.IntentAt.Valid || now.Sub(s.IntentAt.Time) > 15*time.Minute {
				// Harvest retains the submission and its Job for much longer than the
				// Job can execute. Past the reconciliation horizon, restart this page.
				return d.retry(ctx, q, s, "submission_restarted", 300)
			}
			var job intakeJob
			in := struct {
				Request request `json:"request"`
			}{req}
			if err := d.remote.callWithKey(ctx, d.remote.writer, http.MethodPost, "jobs", nil, in, &job, http.StatusAccepted, "gofurry-catalog-"+s.IntentID.String()); err != nil {
				return err
			}
			if !sameIntakeJob(job, req, s.Namespace) {
				return errRemoteConflict
			}
			return q.SaveSteamDiscoveryJob(ctx, dbgen.SaveSteamDiscoveryJobParams{Namespace: s.Namespace, JobID: &job.ID})
		}
		var j intakeJobDetail
		err = d.remote.call(ctx, d.remote.reader, http.MethodGet, "jobs/"+s.JobID.String(), nil, nil, &j, 200)
		if errors.Is(err, remoteStatus(404)) && now.Sub(s.IntentAt.Time) > time.Hour {
			return d.retry(ctx, q, s, "job_missing", 300)
		}
		if err != nil {
			return err
		}
		if !sameIntakeJob(j.Job, req, s.Namespace) || j.Job.ID != *s.JobID || len(j.Attempts) > 3 {
			return errRemoteConflict
		}
		switch j.Job.State {
		case "queued", "running", "retry_wait", "cancel_requested":
			return nil
		case "failed", "cancelled":
			if j.Job.Code == "upstream_auth" {
				return d.retry(ctx, q, s, "upstream_auth", 3600)
			}
			return d.retry(ctx, q, s, "catalog_"+j.Job.State, 300)
		case "succeeded":
		default:
			return errProtocol
		}
		page, err := verifiedDiscoveryPage(j, p, req.Scope, now)
		if err != nil {
			return d.retry(ctx, q, s, "invalid_catalog_page", 300)
		}
		return d.commitPage(ctx, conn, s, pending, page, day)
	})
	if errors.Is(err, errBusy) {
		return nil
	}
	return err
}

func (d *Discovery) retry(ctx context.Context, q *dbgen.Queries, s dbgen.SteamDiscoveryState, code string, delay int32) error {
	return q.RetrySteamDiscoveryPage(ctx, dbgen.RetrySteamDiscoveryPageParams{Namespace: s.Namespace, Code: code, Delay: delay})
}

func (d *Discovery) ensureTarget(ctx context.Context, q *dbgen.Queries, namespace string) error {
	key := targetKey{namespace, "steam", "catalog", "public"}
	var t, confirmed targetView
	if err := d.remote.call(ctx, d.remote.writer, http.MethodPost, "targets", nil, key, &t, 201); err != nil {
		return err
	}
	if t.targetKey != key || !v7(t.ID) || t.State != "active" {
		return errRemoteConflict
	}
	if err := d.remote.call(ctx, d.remote.reader, http.MethodGet, "targets/"+t.ID.String(), nil, nil, &confirmed, 200); err != nil {
		return err
	}
	if t != confirmed {
		return errRemoteConflict
	}
	return q.SaveSteamDiscoveryTarget(ctx, dbgen.SaveSteamDiscoveryTargetParams{Namespace: namespace, TargetID: &t.ID})
}

func verifiedDiscoveryPage(j intakeJobDetail, p discoveryParams, scope scope, now time.Time) (discoveryPage, error) {
	var page discoveryPage
	a, err := acceptedIntakeAttempt(j)
	if err != nil || len(j.Results) != 1 {
		return page, errProtocol
	}
	expected := segmentScope{Namespace: j.Job.Namespace, scope: scope, Dimensions: map[string]string{"section": "catalog"}}
	raw, _ := json.Marshal(expected, json.Deterministic(true))
	digest := sha256.Sum256(raw)
	o := j.Results[0]
	if !acceptedObservation(o, j.Job, a, expected, hex.EncodeToString(digest[:])) || o.State != "present" || o.Evidence != "" ||
		json.Unmarshal(o.Data, &page, json.RejectUnknownMembers(true)) != nil || page.Request != p || !discoveryPageOK(p, page, now) {
		return page, errProtocol
	}
	semantic, _ := json.Marshal(page, json.Deterministic(true))
	evidence, _ := json.Marshal(struct {
		State string
		Data  any
	}{"present", jsontext.Value(semantic)}, json.Deterministic(true))
	hash := sha256.Sum256(evidence)
	if o.Hash != hex.EncodeToString(hash[:]) {
		return page, errProtocol
	}
	return page, nil
}

func discoveryPageOK(p discoveryParams, page discoveryPage, now time.Time) bool {
	if page.Apps == nil || len(page.Apps) > int(p.Limit) {
		return false
	}
	last := p.After
	for _, row := range page.Apps {
		if row.ID <= last || row.Modified <= 0 || row.Modified < int64(p.Since) || row.Modified > now.Add(2*time.Minute).Unix() ||
			len(row.Name) > 192 || row.Name == "" || steamPlainText(row.Name, 192) != row.Name {
			return false
		}
		last = row.ID
	}
	if len(page.Apps) == 0 {
		return !page.More && (page.Last == 0 || page.Last == last)
	}
	return page.Last == last
}

var discoveryTitle = regexp.MustCompile(`(?i)\b(?:furry|fox(?:es)?|wolf|wolves|anthropomorphic|badgers?|otters?|furries)\b|兽人|狐狸|尾巴`)

func (d *Discovery) picks(rows []catalogRow, known []dbgen.SteamDiscoveryKnownRow, day int64) []discoveryPick {
	byKey := make(map[string]dbgen.SteamDiscoveryKnownRow, len(known))
	for _, k := range known {
		byKey[k.ExternalKey] = k
	}
	picked := make([]discoveryPick, 0, len(rows))
	for _, row := range rows {
		status := byKey[strconv.FormatUint(uint64(row.ID), 10)]
		if status.Bound || status.Modified >= row.Modified || status.Reason == "operator_dismissed" {
			continue
		}
		priority := 0
		switch {
		case slices.Contains(d.config.Seeds, row.ID), status.Known:
			priority = 3
		case discoveryTitle.MatchString(row.Name):
			priority = 2
		case d.config.ExplorePerDay > 0:
			input := strconv.AppendUint(nil, uint64(row.ID), 10)
			digest := sha256.Sum256(strconv.AppendInt(append(input, '/'), day, 36))
			if digest[0] < 4 {
				priority = 1
			}
		}
		if priority > 0 {
			picked = append(picked, discoveryPick{row.ID, row.Modified, priority})
		}
	}
	slices.SortStableFunc(picked, func(a, b discoveryPick) int { return b.Priority - a.Priority })
	return picked
}

func (d *Discovery) commitPage(ctx context.Context, conn *pgxpool.Conn, s dbgen.SteamDiscoveryState, pending []discoveryPick, page discoveryPage, day int64) error {
	return pgx.BeginFunc(ctx, conn, func(tx pgx.Tx) error {
		q := dbgen.New(tx)
		keys := make([]string, 0, len(page.Apps))
		for _, row := range page.Apps {
			keys = append(keys, strconv.FormatUint(uint64(row.ID), 10))
		}
		known, err := q.SteamDiscoveryKnown(ctx, keys)
		if err != nil {
			return err
		}
		var explored int32
		already := s.ExploredToday
		if s.BudgetDay != day {
			already = 0
		}
		var added int32
		for _, pick := range d.picks(page.Apps, known, day) {
			if index := slices.IndexFunc(pending, func(p discoveryPick) bool { return p.AppID == pick.AppID }); index >= 0 {
				pending[index].Modified = max(pending[index].Modified, pick.Modified)
				pending[index].Priority = max(pending[index].Priority, pick.Priority)
				continue
			}
			if pick.Priority == 1 {
				if already+explored >= d.config.ExplorePerDay {
					continue
				}
				explored++
			}
			pending = append(pending, pick)
			added++
		}
		raw, err := json.Marshal(pending)
		if err != nil {
			return err
		}
		delay := int32(d.config.PollInterval.Seconds())
		if !page.More {
			delay = int32(d.config.ScanInterval.Seconds())
		}
		return q.CommitSteamDiscoveryPage(ctx, dbgen.CommitSteamDiscoveryPageParams{Namespace: s.Namespace, Pending: raw, LastAppid: int64(page.Last),
			Complete: !page.More, Delay: delay, Day: day, Selected: added, Explored: explored, RowsSeen: int32(len(page.Apps))})
	})
}

func (d *Discovery) drain(ctx context.Context, conn *pgxpool.Conn, namespace string, pending []discoveryPick) ([]discoveryPick, error) {
	slices.SortStableFunc(pending, func(a, b discoveryPick) int { return b.Priority - a.Priority })
	remaining := make([]discoveryPick, 0, len(pending))
	var admitted int32
	err := pgx.BeginFunc(ctx, conn, func(tx pgx.Tx) error {
		q := dbgen.New(tx)
		if err := q.LockIntakeAdmission(ctx); err != nil {
			return err
		}
		day := time.Now().UTC().Unix() / 86400
		used, err := q.SteamDiscoveryAdmissions(ctx, dbgen.SteamDiscoveryAdmissionsParams{Namespace: namespace, Day: day})
		if err != nil {
			return err
		}
		room := min(4, int(d.config.CandidatesPerDay-used))
		for i, pick := range pending {
			if i >= 16 || room <= 0 {
				remaining = append(remaining, pending[i:]...)
				break
			}
			key := strconv.FormatUint(uint64(pick.AppID), 10)
			source, err := intakeSource("steam", key)
			if err != nil {
				return err
			}
			row, err := q.LockIntakeCandidate(ctx, dbgen.LockIntakeCandidateParams{PlatformCode: "steam", ExternalKey: key})
			fresh := errors.Is(err, pgx.ErrNoRows)
			if fresh {
				row, err = d.intake.submit(ctx, q, source, "discovery")
			}
			if errors.Is(err, ErrIntakeCapacity) {
				remaining = append(remaining, pick)
				continue
			}
			if err != nil {
				return err
			}
			if !fresh && row.DiscoveryModified > 0 && pick.Modified > row.DiscoveryModified && row.GameID == nil &&
				(row.State == "resolving" || row.NextAt.Valid && (row.State == "discovered" || row.State == "deferred")) {
				remaining = append(remaining, pick)
				continue
			}
			reopened, err := q.ReopenSteamDiscoveryCandidate(ctx, dbgen.ReopenSteamDiscoveryCandidateParams{ID: row.ID, DiscoveryModified: pick.Modified})
			if err != nil {
				return err
			}
			if err = q.NoteSteamDiscoveryCandidate(ctx, dbgen.NoteSteamDiscoveryCandidateParams{ID: row.ID, DiscoveryModified: pick.Modified}); err != nil {
				return err
			}
			if fresh || reopened == 1 {
				admitted++
				room--
			}
		}
		if len(remaining) == len(pending) {
			return nil
		}
		raw, err := json.Marshal(remaining)
		if err != nil {
			return err
		}
		return q.SaveSteamDiscoveryQueue(ctx, dbgen.SaveSteamDiscoveryQueueParams{Namespace: namespace, Pending: raw, Admitted: admitted, Day: day})
	})
	if err != nil {
		return pending, err
	}
	return remaining, nil
}
