package collection

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"errors"
	"flag"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"
	"uuid"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/librecosta/gofurry/services/api/internal/catalog"
	"github.com/librecosta/gofurry/services/api/internal/database"
	"github.com/librecosta/gofurry/services/api/internal/database/dbgen"
	"github.com/librecosta/gofurry/services/api/internal/platform"
)

var steamMediaSamples = flag.String("steam-media-samples", "", "Private normalized Harvest sample directory")
var steamMediaDB = flag.String("steam-media-db-config", "", "Explicit isolated Steam media test database YAML")

func TestSteamOfferVersions(t *testing.T) {
	p := Plan{Capability: intakeCapability, Interval: time.Hour, Anchor: time.Now(), Locale: "en", Region: "HK", EgressRegion: "cn"}
	params, scoped, policy, err := p.params("app", "434000")
	requirePlanTest(t, err)
	target := uuid.NewV7()
	old, err := planSpec(&target, p.Capability, 1, 1, params, scoped, 3600, p.Anchor, policy)
	requirePlanTest(t, err)
	v2, err := planSpec(&target, p.Capability, 1, 2, params, scoped, 3600, p.Anchor, policy)
	requirePlanTest(t, err)
	oldWire, _ := json.Marshal(old.Request)
	nextWire, _ := json.Marshal(v2.Request)
	if strings.Contains(string(oldWire), "output_schema") || !strings.Contains(string(nextWire), `"output_schema":2`) || sameJobRequest(old.Request, v2.Request) {
		t.Fatal("Schedule identity or default changed")
	}
	p.Capability = "steam.assets"
	if _, _, _, err = p.params("app", "434000"); err == nil {
		t.Fatal("accepted under six-hour cadence")
	}
	p.Interval, p.OutputSchema = 6*time.Hour, 2
	if _, _, _, err = p.params("app", "434000"); err == nil {
		t.Fatal("accepted unknown assets version")
	}
	p.OutputSchema = 1
	_, _, _, err = p.params("app", "434000")
	requirePlanTest(t, err)
}

func TestSteamMediaWireValidation(t *testing.T) {
	base := jsontext.Value(`{"app_id":434000,"images":{},"movies":[{"movie_id":"9007199254740993","name":"Test","order":1,"sources":{"hls_h264":"https://video.steamstatic.com/t.m3u8","dash_h264":"https://video.steamstatic.com/t.mpd"}}],"screenshots":[{"screenshot_id":"0","full_url":"https://shared.steamstatic.com/t.png","order":2}],"truncated":true}`)
	o := observation{identity: identity{Capability: intakeCapability, Schema: 2}, Data: base, State: "present",
		Scope:                 segmentScope{scope: scope{Locale: "en", Region: "us", EgressRegion: "cn"}, Dimensions: map[string]string{"section": "media"}},
		ExecutionEgressRegion: "hk"}
	c, err := normalizeSteam(o, "434000")
	requirePlanTest(t, err)
	if !sameParams(c.Data, base) {
		t.Fatal("structured projection changed wire facts")
	}
	for _, invalid := range []string{
		strings.Replace(string(base), "video.steamstatic.com", "video.steamstatic.com.example", 1),
		strings.Replace(string(base), "t.m3u8", "t.mp4", 1),
		strings.Replace(string(base), `"truncated":true`, `"truncated":true,"unknown":true`, 1),
		strings.Replace(string(base), `"movie_id":"9007199254740993"`, `"movie_id":9007199254740993`, 1),
		strings.Repeat(" ", steamMediaBytes) + string(base),
	} {
		o.Data = jsontext.Value(invalid)
		if _, err := normalizeSteam(o, "434000"); err == nil {
			t.Fatal("accepted invalid known payload")
		}
	}
	o.Data = base
	if _, err := normalizeSteam(o, "1332010"); !errors.Is(err, errMediaIdentity) {
		t.Fatal("wrong AppID was not distinguished from malformed media")
	}
	o.Schema = 4
	if !errors.Is(observationSchema(o), errSchema) {
		t.Fatal("accepted unknown version")
	}
	o.Capability = "steam.unknown"
	if !errors.Is(observationSchema(o), errProtocol) {
		t.Fatal("accepted unknown capability")
	}
}

func privateSteamMedia(t *testing.T, app, kind string) jsontext.Value {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(*steamMediaSamples, app+"-"+kind+".json"))
	requirePlanTest(t, err)
	return jsontext.Value(data)
}

type steamMediaFixture struct {
	mu                     sync.Mutex
	consumer, epoch, reset uuid.UUID
	head, ack              string
	current                []currentItem
	changes                []event
	needReset              bool
	unavailable            bool
}

func (f *steamMediaFixture) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.unavailable {
		w.WriteHeader(503)
		return
	}
	var data any
	switch {
	case strings.HasSuffix(r.URL.Path, "/reset"):
		f.needReset = false
		f.reset = uuid.NewV7()
		data = resetView{ID: f.reset, Epoch: f.epoch, Baseline: f.head, Deadline: time.Now().Add(time.Minute)}
	case strings.HasSuffix(r.URL.Path, "/snapshots/current"):
		data = currentPage{Items: f.current, Epoch: f.epoch, Head: f.head}
	case strings.HasSuffix(r.URL.Path, "/observations/current"):
		data = currentPage{Items: f.current, Epoch: f.epoch, Head: f.head}
	case strings.HasSuffix(r.URL.Path, "/changes"):
		if f.needReset {
			w.WriteHeader(410)
			return
		}
		data = changesPage{Epoch: f.epoch, Next: f.head, Head: f.head}
		if r.URL.Query().Get("after") != f.head {
			data = changesPage{Epoch: f.epoch, Next: f.head, Head: f.head, Items: f.changes}
		}
	case strings.HasSuffix(r.URL.Path, "/ack"):
		var in ackInput
		if json.UnmarshalRead(r.Body, &in) != nil || in.Cursor != f.head ||
			(in.Reset != nil && *in.Reset != f.reset) {
			w.WriteHeader(400)
			return
		}
		f.ack = in.Cursor
		data = ackView{ID: f.consumer, Stream: in.Stream, Epoch: f.epoch, ACK: in.Cursor}
	default:
		w.WriteHeader(404)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.MarshalWrite(w, data)
}

func mediaFixtureEvent(ns, capability, section, locale, country, requested, actual string, schema int32, target uuid.UUID, pos int64, at time.Time, data jsontext.Value) event {
	s := segmentScope{Namespace: ns, scope: scope{Locale: locale, Region: country, EgressRegion: requested}, Dimensions: map[string]string{"section": section}}
	raw, _ := json.Marshal(s, json.Deterministic(true))
	hash := sha256.Sum256(raw)
	content := sha256.Sum256(data)
	id := identity{TargetID: target, Capability: capability, Schema: schema, ScopeKey: hex.EncodeToString(hash[:])}
	o := &observation{identity: id, Scope: s, ExecutionEgressRegion: actual, ID: uuid.NewV7(), State: "present",
		ObservedAt: at, ReceivedAt: at, Data: data, Hash: hex.EncodeToString(content[:]), Source: "steam"}
	return event{identity: id, Position: pos, Action: "upsert", Committed: at, Observation: o}
}

func TestSteamMediaNative(t *testing.T) {
	if *steamMediaDB == "" || *steamMediaSamples == "" {
		t.Skip("explicit isolated database and private Harvest samples required")
	}
	ctx := t.Context()
	cfg, err := platform.LoadConfig(*steamMediaDB)
	requirePlanTest(t, err)
	if cfg.Database == nil || cfg.Database.Host != "127.0.0.1" || cfg.Database.Name != "gofurry_r1c_media_test" {
		t.Fatal("only the designated isolated loopback database is supported")
	}
	pool, err := database.Open(ctx, *cfg.Database)
	requirePlanTest(t, err)
	defer pool.Close()
	q := dbgen.New(pool)
	consumer := uuid.NewV7()
	ns := "r1c-" + strings.ReplaceAll(consumer.String(), "-", "")
	f := &steamMediaFixture{consumer: consumer, epoch: uuid.NewV7(), head: "baseline"}
	up := httptest.NewServer(f)
	defer up.Close()
	svc := New(pool, platform.HarvestConfig{URL: up.URL, Namespace: ns, ConsumerID: consumer.String(), ReaderToken: "test-reader", WriterToken: "test-bind", PageSize: 32})
	defer svc.Close()
	editor := catalog.New(pool)
	type title struct {
		game           dbgen.Game
		source, target uuid.UUID
		market         string
		media, assets  event
	}
	games := make(map[string]title)
	start := time.Now()
	var pos int64
	for _, sample := range []struct{ app, name, market string }{
		{"434000", "Paws — A Shelter 2 Game", "hk"}, {"1332010", "Stray", "cn"},
	} {
		g, err := editor.CreateGame(ctx, catalog.GameInput{
			Editorial: catalog.Editorial{Slug: "r1c-" + sample.app, Localizations: []catalog.Localization{{Locale: "en", Name: sample.name}, {Locale: "zh", Name: sample.name}}},
			Sources:   []catalog.Source{{PlatformCode: "steam", ExternalKey: sample.app}},
		})
		requirePlanTest(t, err)
		b, err := svc.BindGame(ctx, g.PublicID, sample.app)
		requirePlanTest(t, err)
		target := uuid.NewV7()
		requirePlanTest(t, q.SaveCollectionTarget(ctx, dbgen.SaveCollectionTargetParams{ID: b.SourceID, TargetID: &target}))
		for _, p := range []Plan{
			{Capability: intakeCapability, OutputSchema: 1, Locale: "en", Region: sample.market, EgressRegion: "cn"},
			{Capability: intakeCapability, OutputSchema: 2, Locale: "en", Region: sample.market, EgressRegion: "cn"},
			{Capability: "steam.assets", OutputSchema: 1, Locale: "en", Region: "us", EgressRegion: "cn"},
			{Capability: "steam.assets", OutputSchema: 1, Locale: "zh-cn", Region: "hk", EgressRegion: "cn"},
			{Capability: "steam.assets", OutputSchema: 1, Locale: "en", Region: "cn", EgressRegion: "cn"},
		} {
			p.Anchor, p.Interval = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), 24*time.Hour
			_, err := svc.AddPlan(ctx, b.ID, p)
			requirePlanTest(t, err)
		}
		at := time.Now().UTC().Truncate(time.Microsecond)
		add := func(capability, section, market string, version int32, data jsontext.Value) event {
			pos++
			ev := mediaFixtureEvent(ns, capability, section, "en", market, "cn", "cn", version, target, pos, at, data)
			f.current = append(f.current, currentItem{observation: *ev.Observation, Position: ev.Position, Confirmed: at})
			return ev
		}
		add(intakeCapability, "media", sample.market, 1, privateSteamMedia(t, sample.app, "media-v1"))
		add(intakeCapability, "details", sample.market, 2, privateSteamMedia(t, sample.app, "details-v2"))
		add(intakeCapability, "price", sample.market, 2, privateSteamMedia(t, sample.app, "price-v2"))
		media := add(intakeCapability, "media", sample.market, 2, privateSteamMedia(t, sample.app, "media-v2"))
		var realAsset struct {
			ObservedAt time.Time      `json:"observed_at"`
			Data       jsontext.Value `json:"data"`
		}
		requirePlanTest(t, json.Unmarshal(privateSteamMedia(t, sample.app, "assets-v1"), &realAsset))
		assets := add("steam.assets", "assets", "us", 1, realAsset.Data)
		assets.Observation.ObservedAt = realAsset.ObservedAt
		f.current[len(f.current)-1].observation = *assets.Observation
		games[sample.app] = title{g, *b.GameSourceID, target, sample.market, media, assets}
	}
	svc.remote.writer = ""
	requirePlanTest(t, svc.Tick(ctx))
	t.Logf("two games, approved versioned Plans and snapshot activation %s", time.Since(start))
	state, err := q.GetCollectionConsumer(ctx, ns)
	requirePlanTest(t, err)
	if state.Cursor != "baseline" || state.AckPending {
		t.Fatal("snapshot did not activate and ACK")
	}
	var quote int
	requirePlanTest(t, pool.QueryRow(ctx, "SELECT count(*) FROM game_channel_quotes('hk','HKD','cn') WHERE game_id=$1",
		games["434000"].game.ID).Scan(&quote))
	if quote != 1 {
		t.Fatal("the market quote did not use the approved v2 price Plan")
	}
	for _, check := range []struct {
		app                   string
		movies, shots, assets int
	}{{"434000", 2, 12, 9}, {"1332010", 3, 8, 15}} {
		g := games[check.app]
		view, err := editor.ManagementGame(ctx, g.game.PublicID)
		requirePlanTest(t, err)
		if len(view.MediaCandidatesV2) != 2 || !slices.ContainsFunc(view.SourceCandidates, func(c catalog.GameSourceCandidate) bool { return c.Section == "media" }) {
			t.Fatal("v1 and v2/Assets did not coexist")
		}
		for _, c := range view.MediaCandidatesV2 {
			switch {
			case c.Media != nil:
				raw, _ := json.Marshal(c.Media)
				if len(c.Media.Movies) != check.movies || len(c.Media.Screenshots) != check.shots || !sameParams(raw, g.media.Observation.Data) {
					t.Fatal("high-fidelity media fields or original order were lost")
				}
			case c.Assets != nil:
				raw, _ := json.Marshal(c.Assets)
				if len(c.Assets.Assets) != check.assets || !sameParams(raw, g.assets.Observation.Data) {
					t.Fatal("StoreBrowse source fields were lost")
				}
			default:
				t.Fatal("untyped candidate")
			}
		}
		t.Logf("app=%s movies=%d screenshots=%d assets=%d", check.app, check.movies, check.shots, check.assets)
	}

	paws, stray := games["434000"], games["1332010"]
	view, err := editor.ManagementGame(ctx, paws.game.PublicID)
	requirePlanTest(t, err)
	m := view.MediaCandidatesV2[0]
	a := view.MediaCandidatesV2[1]
	if m.Media == nil {
		m, a = a, m
	}
	pick := func(c catalog.GameMediaCandidateV2, id string) catalog.GameMediaPick {
		return catalog.GameMediaPick{GameSourceID: c.GameSourceID, ObservationID: c.ObservationID, ID: id}
	}
	choices := catalog.GameMediaLocaleEdit{Locale: "en", Images: []catalog.GameMediaPick{pick(m, "header_url")},
		Screenshots: []catalog.GameMediaPick{pick(m, m.Media.Screenshots[3].ID), pick(m, m.Media.Screenshots[0].ID)},
		Movies:      []catalog.GameMediaPick{pick(m, m.Media.Movies[0].ID)}}
	cover := pick(a, a.Assets.Assets[0].ID)
	choices.Cover = &cover
	edit := catalog.GameMediaEditV2{Revision: view.Revision, Selections: []catalog.GameMediaLocaleEdit{choices}}
	forged := edit
	forged.Selections = slices.Clone(edit.Selections)
	forged.Selections[0].Movies = []catalog.GameMediaPick{{GameSourceID: stray.source, ObservationID: stray.media.Observation.ID, ID: m.Media.Movies[0].ID}}
	if _, err = editor.SaveManagementGameMediaV2(ctx, paws.game.PublicID, forged); !errors.Is(err, catalog.ErrInvalid) {
		t.Fatal("accepted cross-game selection")
	}
	timing := time.Now()
	draft, err := editor.SaveManagementGameMediaV2(ctx, paws.game.PublicID, edit)
	requirePlanTest(t, err)
	t.Logf("mixed-origin draft write %s", time.Since(timing))
	view, err = editor.ManagementGame(ctx, paws.game.PublicID)
	requirePlanTest(t, err)
	if len(view.DraftMediaV2) != 1 || view.PreviewToken == "" || view.DraftMediaV2[0].Media.Screenshots[0].Order != 3 || draft.Revision != view.Revision {
		t.Fatal("draft selection, revision or custom ordering lost")
	}
	oldToken := view.PreviewToken
	legacy, err := editor.SaveManagementGame(ctx, paws.game.PublicID, catalog.GameManagementEdit{
		Revision: view.Revision, Localizations: []catalog.Localization{{Locale: "en", Name: "Paws edited"}, {Locale: "zh", Name: "Paws"}}, DraftMedia: []catalog.GameMediaSelection{},
	})
	requirePlanTest(t, err)
	view, err = editor.ManagementGame(ctx, paws.game.PublicID)
	requirePlanTest(t, err)
	if len(view.DraftMediaV2) != 1 || view.Revision != legacy.Revision || oldToken == view.PreviewToken {
		t.Fatal("legacy edit erased structured data or preview did not track changes")
	}
	publish := func(view catalog.GameManagement, changes bool) error {
		action := "game.publish"
		if changes {
			action = "game.publish_changes"
		}
		_, err := editor.Manage(ctx, dbgen.RecordManagementOperationParams{RequestID: uuid.NewV7(), ActorID: uuid.NewV7(), ActorName: "Media test", ActorRole: "operator", Action: action},
			func(ctx context.Context, s *catalog.Service, _ *dbgen.Queries) (catalog.ManagementResult, error) {
				in := catalog.GamePublicationReview{Revision: view.Revision, ConfirmRelevance: true, ContentRating: "sfw", PreviewToken: view.PreviewToken}
				if changes {
					return s.PublishManagementGameChanges(ctx, paws.game.PublicID, in)
				}
				return s.PublishManagementGame(ctx, paws.game.PublicID, in)
			})
		return err
	}
	market := catalog.ChannelMarket{Region: "CN", Egress: "cn"}
	if _, err := editor.PublicGameProfile(ctx, market, paws.game.PublicID, "en"); !errors.Is(err, catalog.ErrNotFound) {
		t.Fatalf("draft Game exposed its media: %v", err)
	}
	readPublishedMedia := func() *catalog.PublicGameMedia {
		t.Helper()
		profile, err := editor.PublicGameProfile(ctx, market, paws.game.PublicID, "en")
		requirePlanTest(t, err)
		if profile.PublishedMedia == nil || profile.PublicID != paws.game.PublicID || profile.Slug != paws.game.Slug {
			t.Fatal("public identity or media changed")
		}
		return profile.PublishedMedia
	}
	encodedPublishedMedia := func() []byte {
		t.Helper()
		data, err := json.Marshal(readPublishedMedia())
		requirePlanTest(t, err)
		return data
	}
	badPreview := view
	badPreview.PreviewToken = oldToken
	if !errors.Is(publish(badPreview, false), catalog.ErrGamePreviewChanged) {
		t.Fatal("accepted preview token with stale draft")
	}
	requirePlanTest(t, publish(view, false))
	view, err = editor.ManagementGame(ctx, paws.game.PublicID)
	requirePlanTest(t, err)
	if len(view.PublishedMediaV2) != 1 || view.HasUnpublishedChanges {
		t.Fatal("publication did not copy the independently stored media")
	}
	original := view.PublishedMediaV2[0].Media
	size, _ := json.Marshal(original)
	t.Logf("published snapshot %d bytes, game=%s", len(size), paws.game.PublicID)
	publishedProfile, err := editor.PublicGameProfile(ctx, market, paws.game.PublicID, "en")
	requirePlanTest(t, err)
	if publishedProfile.CoverURL != original.Cover.URL {
		t.Fatal("public profile did not project the adopted cover")
	}
	for _, source := range publishedProfile.Sources {
		if source.Media != nil && len(source.Media.Movies) != 0 {
			t.Fatal("published a manifest as a direct movie URL")
		}
	}

	publicMedia := readPublishedMedia()
	if publicMedia.Version != 2 || publicMedia.Locale != "en" || publicMedia.Fallback ||
		publicMedia.Cover == nil || publicMedia.Cover.Type != original.Cover.Type || publicMedia.Cover.URL != original.Cover.URL ||
		len(publicMedia.Images) != 1 || publicMedia.Images[0].Type != "header" ||
		len(publicMedia.Screenshots) != 2 || publicMedia.Screenshots[0].ID != original.Screenshots[0].ID ||
		len(publicMedia.Movies) != 1 || publicMedia.Movies[0].ID != original.Movies[0].ID ||
		publicMedia.Movies[0].Sources != original.Movies[0].Sources {
		t.Fatal("public projection lost adopted media, order or formats")
	}
	initialMedia := encodedPublishedMedia()

	at := time.Now().UTC()
	pos++
	newData := jsontext.Value(strings.Replace(string(paws.media.Observation.Data), `"truncated":false`, `"truncated":true`, 1))
	newMedia := mediaFixtureEvent(ns, intakeCapability, "media", "en", paws.market, "cn", "intl", 2, paws.target, pos, at, newData)
	pos++
	empty := mediaFixtureEvent(ns, "steam.assets", "assets", "en", "us", "cn", "cn", 1, paws.target, pos, at,
		jsontext.Value(`{"app_id":434000,"assets":[],"truncated":false}`))
	empty.Observation.State, empty.Observation.Evidence = "confirmed_empty", "store_assets_empty"
	pos++
	localized := mediaFixtureEvent(ns, "steam.assets", "assets", "zh-cn", "hk", "cn", "cn", 1, paws.target, pos, at,
		jsontext.Value(`{"app_id":434000,"assets":[],"truncated":false}`))
	localized.Observation.State, localized.Observation.Evidence = "confirmed_empty", "store_assets_empty"
	pos++
	var assetsInCN catalog.SteamAssetsV1
	requirePlanTest(t, json.Unmarshal(paws.assets.Observation.Data, &assetsInCN))
	for i := range assetsInCN.Assets {
		assetsInCN.Assets[i].ObservedAt = at
	}
	marketData, err := json.Marshal(assetsInCN)
	requirePlanTest(t, err)
	otherMarket := mediaFixtureEvent(ns, "steam.assets", "assets", "en", "cn", "cn", "cn", 1, paws.target, pos, at, marketData)
	pos++
	malformed := mediaFixtureEvent(ns, intakeCapability, "media", "en", paws.market, "cn", "cn", 2, paws.target, pos, at, jsontext.Value(`{"app_id":434000,"movies":[]}`))
	increment := time.Now()
	f.mu.Lock()
	f.head, f.changes = "incremental", []event{newMedia, empty, localized, otherMarket, malformed}
	f.mu.Unlock()
	requirePlanTest(t, svc.Tick(ctx))
	t.Logf("five-event incremental page and ACK %s", time.Since(increment))
	state, err = q.GetCollectionConsumer(ctx, ns)
	requirePlanTest(t, err)
	if state.Cursor != "incremental" || state.AckPending {
		t.Fatal("incremental persistence not committed with cursor")
	}
	view, err = editor.ManagementGame(ctx, paws.game.PublicID)
	requirePlanTest(t, err)
	if len(view.MediaCandidatesV2) != 5 || len(view.PublishedMediaV2) != 1 || view.HasUnpublishedChanges ||
		!slices.ContainsFunc(view.MediaCandidatesV2, func(c catalog.GameMediaCandidateV2) bool {
			return c.Locale == "zh-cn" && c.StoreCountry == "hk" && c.State == "confirmed_empty"
		}) ||
		!slices.ContainsFunc(view.MediaCandidatesV2, func(c catalog.GameMediaCandidateV2) bool {
			return c.StoreCountry == "cn" && c.State == "present" && c.Assets != nil
		}) ||
		!slices.ContainsFunc(view.MediaCandidatesV2, func(c catalog.GameMediaCandidateV2) bool {
			return c.EgressRegion == "intl" && c.Media != nil && *c.Media.Truncated
		}) {
		t.Fatal("empty or truncated scope overwrote unrelated source or editorial media")
	}
	confirmed := time.Now().UTC().Truncate(time.Microsecond)
	f.mu.Lock()
	f.current = []currentItem{{observation: *newMedia.Observation, Position: newMedia.Position, Confirmed: confirmed}}
	f.mu.Unlock()
	requirePlanTest(t, q.SaveCollectionSweep(ctx, dbgen.SaveCollectionSweepParams{Namespace: ns, SweepAt: timestamp(confirmed.Add(-time.Minute))}))
	requirePlanTest(t, svc.Tick(ctx))
	view, err = editor.ManagementGame(ctx, paws.game.PublicID)
	requirePlanTest(t, err)
	if !slices.ContainsFunc(view.MediaCandidatesV2, func(c catalog.GameMediaCandidateV2) bool {
		return c.ObservationID == newMedia.Observation.ID && c.ConfirmedAt.Equal(confirmed)
	}) || len(view.MediaCandidatesV2) != 5 {
		t.Fatal("same Observation confirmation changed its identity")
	}

	// Reducing and reordering saved picks needs no active source for those picks.
	choices.Cover = nil
	choices.Screenshots[0], choices.Screenshots[1] = choices.Screenshots[1], choices.Screenshots[0]
	edit.Revision, edit.Selections = view.Revision, []catalog.GameMediaLocaleEdit{choices}
	_, err = editor.SaveManagementGameMediaV2(ctx, paws.game.PublicID, edit)
	requirePlanTest(t, err)
	view, err = editor.ManagementGame(ctx, paws.game.PublicID)
	requirePlanTest(t, err)
	if !view.HasUnpublishedChanges || view.DraftMediaV2[0].Media.Cover != nil || view.PublishedMediaV2[0].Media.Cover == nil {
		t.Fatal("draft and publication changed together")
	}
	if !bytes.Equal(initialMedia, encodedPublishedMedia()) {
		t.Fatal("new candidate observations or editing changed the public media before publication")
	}
	requirePlanTest(t, publish(view, true))
	changed := readPublishedMedia()
	if changed.Cover != nil || len(changed.Screenshots) != 2 || changed.Screenshots[0].ID != original.Screenshots[1].ID {
		t.Fatal("public media did not update after Publish Changes")
	}
	currentMedia := encodedPublishedMedia()

	f.mu.Lock()
	f.epoch, f.needReset, f.head, f.current, f.changes = uuid.NewV7(), true, "new-epoch", nil, nil
	f.mu.Unlock()
	requirePlanTest(t, svc.Tick(ctx))
	state, err = q.GetCollectionConsumer(ctx, ns)
	requirePlanTest(t, err)
	if state.Cursor != "new-epoch" || state.AckPending {
		t.Fatal("410 recovery did not commit new epoch")
	}
	f.mu.Lock()
	f.unavailable = true
	f.mu.Unlock()
	if err := svc.Tick(ctx); err == nil {
		t.Fatal("missing upstream availability result")
	}
	latest, err := editor.ManagementGame(ctx, paws.game.PublicID)
	requirePlanTest(t, err)
	if len(latest.MediaCandidatesV2) != 5 || len(latest.DraftMediaV2) != 1 || len(latest.PublishedMediaV2) != 1 || latest.HasUnpublishedChanges ||
		latest.PublishedMediaV2[0].Media.Movies[0].Sources != original.Movies[0].Sources {
		t.Fatal("reset, outage or publish changes lost durable media")
	}
	profile, err := editor.PublicGameProfile(ctx, market, paws.game.PublicID, "en")
	requirePlanTest(t, err)
	if profile.Slug != paws.game.Slug || !bytes.Equal(currentMedia, encodedPublishedMedia()) {
		t.Fatal("reset or outage changed published media or identity")
	}
	for _, bad := range []struct {
		e   event
		err error
	}{
		{mediaFixtureEvent(ns, intakeCapability, "media", "en", paws.market, "cn", "cn", 3, paws.target, pos+1, time.Now().UTC(), paws.media.Observation.Data), errSchema},
		{mediaFixtureEvent(ns, intakeCapability, "media", "en", paws.market, "cn", "cn", 2, paws.target, pos+1, time.Now().UTC(), stray.media.Observation.Data), errMediaIdentity},
	} {
		f.mu.Lock()
		f.unavailable, f.head, f.changes = false, "protocol-error", []event{bad.e}
		f.mu.Unlock()
		if err := svc.Tick(ctx); !errors.Is(err, bad.err) {
			t.Fatalf("unexpected protocol failure %v", err)
		}
		state, err = q.GetCollectionConsumer(ctx, ns)
		requirePlanTest(t, err)
		if state.Cursor != "new-epoch" || state.AckPending {
			t.Fatal("ACKed unsupported or misattributed observation")
		}
	}
	// PostgreSQL write failures roll back both cursor and source projection.
	f.mu.Lock()
	f.unavailable, f.head, f.changes = false, "failed", []event{newMedia}
	f.mu.Unlock()
	_, err = pool.Exec(ctx, `ALTER TABLE collection_current ADD CONSTRAINT r1c_fail_write CHECK (position<0) NOT VALID`)
	requirePlanTest(t, err)
	defer func() {
		_, _ = pool.Exec(context.Background(), "ALTER TABLE collection_current DROP CONSTRAINT IF EXISTS r1c_fail_write")
	}()
	if err := svc.Tick(ctx); err == nil || errors.Is(err, pgx.ErrNoRows) {
		t.Fatal("expected source write error")
	}
	state, err = q.GetCollectionConsumer(ctx, ns)
	requirePlanTest(t, err)
	if state.Cursor != "new-epoch" || state.AckPending {
		t.Fatal("cursor advanced without source projection")
	}
	_, err = pool.Exec(ctx, "ALTER TABLE collection_current DROP CONSTRAINT r1c_fail_write")
	requirePlanTest(t, err)
	requirePlanTest(t, svc.Tick(ctx))
	steamGameMediaQueryScale(t, pool, paws.game.ID, paws.source)
	withdraw := event{identity: newMedia.identity, Position: pos + 1, Action: "withdraw",
		Committed: time.Now().UTC(), Tombstone: newMedia.Observation.Scope}
	f.mu.Lock()
	f.head, f.changes = "withdrawal", []event{withdraw}
	f.mu.Unlock()
	requirePlanTest(t, svc.Tick(ctx))
	latest, err = editor.ManagementGame(ctx, paws.game.PublicID)
	requirePlanTest(t, err)
	if len(latest.MediaCandidatesV2) != 3 || len(latest.PublishedMediaV2) != 1 ||
		slices.ContainsFunc(latest.MediaCandidatesV2, func(c catalog.GameMediaCandidateV2) bool { return c.Media != nil }) {
		t.Fatal("scope withdrawal did not hide all declared exits or changed the publication")
	}
	if !bytes.Equal(currentMedia, encodedPublishedMedia()) {
		t.Fatal("withdrawing candidates changed published media")
	}
}

func steamGameMediaQueryScale(t *testing.T, pool *pgxpool.Pool, game, source uuid.UUID) {
	t.Helper()
	ctx := t.Context()
	for _, count := range []int{100, 1000, 10000} {
		func() {
			tx, err := pool.Begin(ctx)
			requirePlanTest(t, err)
			defer tx.Rollback(ctx)
			_, err = tx.Exec(ctx, `
WITH real AS (SELECT * FROM game_source_media WHERE game_source_id=$1 AND state='present' LIMIT 1)
INSERT INTO game_source_media (game_source_id,collection_source_id,binding_id,target_id,namespace,capability,output_schema,
 locale,store_country,egress_region,requested_egress_region,scope_key,epoch,position,event_at,state,observation_id,content_hash,ruleset,observed_at,confirmed_at,payload)
 SELECT r.game_source_id,r.collection_source_id,r.binding_id,r.target_id,r.namespace,r.capability,r.output_schema,
  r.locale,r.store_country,r.egress_region,r.requested_egress_region,encode(sha256(i::text::bytea),'hex'),
  r.epoch,r.position,r.event_at,r.state,uuidv7(),r.content_hash,r.ruleset,r.observed_at,r.confirmed_at,r.payload
 FROM real r CROSS JOIN generate_series(1,$2::integer-(SELECT count(*)::integer FROM game_source_media WHERE game_source_id=$1)) i`, source, count)
			requirePlanTest(t, err)
			q := dbgen.New(tx)
			args := dbgen.ListGameSourceMediaParams{GameID: game, TakeRows: 13}
			first, err := q.ListGameSourceMedia(ctx, args)
			requirePlanTest(t, err)
			args.AfterID = first[11].ID
			timings := make([]time.Duration, 0, 25)
			for range 25 {
				start := time.Now()
				rows, err := q.ListGameSourceMedia(ctx, args)
				requirePlanTest(t, err)
				if len(rows) != 13 || rows[0].ID != first[12].ID {
					t.Fatal("invalid or unstable keyset")
				}
				timings = append(timings, time.Since(start))
			}
			slices.Sort(timings)
			t.Logf("rows=%d bounded 13-row SQL page median=%s", count, timings[len(timings)/2])
		}()
	}
}
