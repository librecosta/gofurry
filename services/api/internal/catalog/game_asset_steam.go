package catalog

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	json "encoding/json/v2"
	"errors"
	"net/url"
	"path"
	"strconv"
	"strings"
	"time"
	"uuid"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/librecosta/gofurry/services/api/internal/database/dbgen"
)

type sourceAsset struct {
	Kind      string          `json:"kind"`
	Category  string          `json:"category"`
	Role      string          `json:"role"`
	Identity  string          `json:"identity"`
	Name      string          `json:"name"`
	Order     *int            `json:"original_order"`
	Highlight *bool           `json:"highlight"`
	Variants  []sourceVariant `json:"variants"`
}

type sourceVariant struct {
	Key    string `json:"key"`
	URL    string `json:"url"`
	Format string `json:"format"`
	Role   string `json:"role"`
	ID     string `json:"source_id"`
	Slot   string `json:"slot"`
	Digest string `json:"digest"`
	Family string `json:"family"`
	Order  *int   `json:"source_order"`
}

func (v sourceVariant) withURL(key, raw, extension string) sourceVariant {
	v.Key, v.URL = key, publicMediaURL(raw, extension)
	if v.URL != "" {
		// publicMediaURL already checked the path and extension.
		u, _ := url.Parse(v.URL)
		v.Format = strings.ToLower(strings.TrimPrefix(path.Ext(u.Path), "."))
	}
	return v
}

func (a *sourceAsset) add(v sourceVariant) {
	if v.URL != "" {
		a.Variants = append(a.Variants, v)
	}
}

func steamArtwork(img GameSelectedImage, app string, index *int) sourceAsset {
	a := sourceAsset{Kind: "image", Category: "artwork", Role: img.Type, Identity: "slot"}
	public, ok := publicMediaImage(img)
	generatedBackground := img.Origin.Capability == "steam.details" && img.Family == "store" &&
		(img.Type == "background" || img.Type == "background_raw") &&
		img.URL == "https://store.akamai.steamstatic.com/images/storepagebackground/app/"+app
	if (!ok && !generatedBackground) || img.ID == "" || img.Slot == "" {
		return a
	}
	v := sourceVariant{Role: img.Type, ID: img.ID, Slot: img.Slot, Family: img.Family, Order: index}
	key := "standard"
	switch img.Origin.Capability {
	case "steam.assets":
		if img.Asset == nil || img.Origin.OutputSchema != 1 {
			return a
		}
		s := img.Asset
		sum := sha256.Sum256([]byte(strings.Join([]string{app, s.Slot, s.Locale, s.URL}, "\x00")))
		if !steamArtworkPath(s, app) || s.ID != img.ID || s.ID != hex.EncodeToString(sum[:]) || s.Type != img.Type || s.Slot != img.Slot ||
			s.Family != img.Family || s.URL != img.URL || s.Locale != img.Origin.Locale || s.Source != "store_browse" ||
			s.ObservedAt.Sub(img.Origin.ObservedAt).Abs() > time.Microsecond {
			return a
		}
		v.Digest = s.Digest
		// A declared StoreBrowse second-density slot shares a stable logical role.
		if base, ok := strings.CutSuffix(img.Type, "_2x"); ok {
			a.Role, key = base, "2x"
		}
	case "steam.details":
		if img.Origin.OutputSchema < 2 || img.Origin.OutputSchema > 3 || img.Asset != nil || img.ID != img.Slot || img.Slot != img.Type+"_url" {
			return a
		}
		switch img.Type {
		case "header", "capsule", "capsule_v5", "background", "background_raw":
		default:
			return a
		}
	default:
		return a
	}
	if generatedBackground {
		v.Key, v.URL = key, img.URL
	} else {
		v = v.withURL(key, public.URL, "")
	}
	a.add(v)
	return a
}

func steamShot(s SteamScreenshot) sourceAsset {
	a := sourceAsset{Kind: "image", Category: "screenshot", Role: "screenshot", Identity: s.ID, Order: &s.Order}
	if publicMediaID(s.ID, true) && s.Order >= 0 {
		v := sourceVariant{Role: a.Role, ID: s.ID, Slot: "thumbnail", Order: a.Order}
		a.add(v.withURL("thumbnail", s.Thumbnail, ""))
		v.Slot = "full"
		a.add(v.withURL("full", s.Full, ""))
	}
	return a
}

func steamFilm(m SteamMovie) sourceAsset {
	a := sourceAsset{Kind: "video", Category: "trailer", Role: "movie", Identity: m.ID, Name: m.Name, Order: &m.Order, Highlight: m.Highlight}
	if !publicMediaID(m.ID, false) || m.Order < 0 || len(m.Name) > 256 || strings.ContainsFunc(m.Name, unsafeMediaRune) {
		return a
	}
	s := m.Sources
	for _, item := range []struct{ key, slot, link, ext string }{
		{"poster", "thumbnail", m.Thumbnail, ""},
		{"mp4_480", "mp4_480", s.MP4480, ".mp4"}, {"mp4_max", "mp4_max", s.MP4Max, ".mp4"},
		{"webm_480", "webm_480", s.WebM480, ".webm"}, {"webm_max", "webm_max", s.WebMMax, ".webm"},
		{"hls_h264", "hls_h264", s.HLSH264, ".m3u8"},
		{"dash_h264", "dash_h264", s.DASHH264, ".mpd"}, {"dash_av1", "dash_av1", s.DASHAV1, ".mpd"},
	} {
		v := sourceVariant{Role: a.Role, ID: m.ID, Slot: item.slot, Order: a.Order}
		a.add(v.withURL(item.key, item.link, item.ext))
	}
	return a
}

func assetSteamBinding(ctx context.Context, q *dbgen.Queries, o GameMediaOrigin) (dbgen.GameAssetSteamBindingRow, error) {
	b, err := q.GameAssetSteamBinding(ctx, dbgen.GameAssetSteamBindingParams{GameSourceID: o.GameSourceID, BindingID: o.BindingID,
		CollectionSourceID: o.CollectionSourceID, TargetID: &o.TargetID, Namespace: o.Namespace})
	if errors.Is(err, pgx.ErrNoRows) {
		err = errAssetAttribution
	}
	return b, err
}

var (
	errAssetAttribution   = errors.New("unverified source attribution")
	errAssetNormalization = errors.New("unrepresentable source asset")
)

func assetSteamSource(ctx context.Context, q *dbgen.Queries, row dbgen.GameSourceMedium) (dbgen.GameAssetSteamBindingRow, []sourceAsset, *bool, error) {
	var b dbgen.GameAssetSteamBindingRow
	c, err := mediaCandidate(row)
	if err != nil {
		return b, nil, nil, errAssetNormalization
	}
	b, err = assetSteamBinding(ctx, q, c.GameMediaOrigin)
	if err != nil {
		return b, nil, nil, err
	}
	items := []sourceAsset{}
	var invalid bool
	add := func(a sourceAsset) {
		if len(a.Variants) == 0 {
			invalid = true
			return
		}
		for i := range items {
			if items[i].Role == a.Role && items[i].Identity == a.Identity {
				items[i].Variants = append(items[i].Variants, a.Variants...)
				return
			}
		}
		items = append(items, a)
	}
	var truncated *bool
	switch {
	case c.Assets != nil && strconv.FormatUint(uint64(c.Assets.AppID), 10) == b.ExternalKey:
		truncated = c.Assets.Truncated
		for i, v := range c.Assets.Assets {
			add(steamArtwork(GameSelectedImage{Origin: c.GameMediaOrigin, ID: v.ID, Type: v.Type, Family: v.Family,
				Slot: v.Slot, URL: v.URL, Asset: &v}, b.ExternalKey, &i))
		}
	case c.Media != nil && c.Media.Images != nil && strconv.FormatUint(uint64(c.Media.AppID), 10) == b.ExternalKey:
		truncated = c.Media.Truncated
		v := c.Media.Images
		for _, item := range []struct{ role, link string }{
			{"header", v.Header}, {"capsule", v.Capsule}, {"capsule_v5", v.CapsuleV5},
			{"background", v.Background}, {"background_raw", v.BackgroundRaw},
		} {
			if item.link != "" {
				slot := item.role + "_url"
				add(steamArtwork(GameSelectedImage{Origin: c.GameMediaOrigin, ID: slot, Type: item.role, Family: "store", Slot: slot, URL: item.link}, b.ExternalKey, nil))
			}
		}
		for _, shot := range c.Media.Screenshots {
			add(steamShot(shot))
		}
		for _, movie := range c.Media.Movies {
			add(steamFilm(movie))
		}
	default:
		return b, nil, nil, errAssetAttribution
	}
	if invalid {
		err = errAssetNormalization
	}
	return b, items, truncated, err
}

func putAssetVersions(ctx context.Context, q *dbgen.Queries, o GameMediaOrigin, externalKey string, items []sourceAsset, truncated *bool) (int, error) {
	if len(items) == 0 {
		return 0, nil
	}
	data, err := json.Marshal(items)
	if err != nil {
		return 0, err
	}
	origin, err := json.Marshal(o)
	if err != nil {
		return 0, err
	}
	trunc := pgtype.Bool{}
	if truncated != nil {
		trunc = pgtype.Bool{Bool: *truncated, Valid: true}
	}
	err = q.PutGameAssets(ctx, dbgen.PutGameAssetsParams{GameSourceID: o.GameSourceID, ExternalKey: externalKey, SourceFamily: o.Capability, Items: data})
	if err != nil {
		return 0, err
	}
	err = q.PutGameAssetVersions(ctx, dbgen.PutGameAssetVersionsParams{
		GameSourceID: o.GameSourceID, ExternalKey: externalKey, SourceFamily: o.Capability, Items: data,
		Origin: origin, BindingID: o.BindingID, ObservationID: o.ObservationID, ScopeKey: o.ScopeKey, EgressRegion: o.EgressRegion, Locale: o.Locale, Truncated: trunc,
	})
	if err != nil {
		return 0, err
	}
	p := dbgen.PutGameAssetVariantsParams{Items: data, GameSourceID: o.GameSourceID, ExternalKey: externalKey, SourceFamily: o.Capability,
		BindingID: o.BindingID, ObservationID: o.ObservationID, ScopeKey: o.ScopeKey, EgressRegion: o.EgressRegion}
	inserted, err := q.PutGameAssetVariants(ctx, p)
	if err != nil {
		return 0, err
	}
	count, err := q.CountGameAssetNormalization(ctx, dbgen.CountGameAssetNormalizationParams(p))
	if err != nil {
		return 0, err
	}
	want := 0
	for _, item := range items {
		want += len(item.Variants)
	}
	if int(count) != want {
		return 0, errAssetAttribution
	}
	return len(inserted), nil
}

// ProjectGameAssetSource runs inside the Consumer transaction that persists the
// existing trusted source projection and its cursor. Historical versions remain.
func ProjectGameAssetSource(ctx context.Context, q *dbgen.Queries, row dbgen.GameSourceMedium) error {
	if row.State != "present" || row.Ruleset != "gofurry-steam-media-2" {
		return nil
	}
	b, items, truncated, err := assetSteamSource(ctx, q, row)
	if err != nil && !errors.Is(err, errAssetNormalization) {
		return err
	}
	o, err := mediaOrigin(row)
	if err != nil {
		return err
	}
	_, err = putAssetVersions(ctx, q, o, b.ExternalKey, items, truncated)
	return err
}

func findAssetVersion(ctx context.Context, q *dbgen.Queries, o GameMediaOrigin, externalKey string, a sourceAsset) (uuid.UUID, error) {
	return q.FindGameAssetVersion(ctx, dbgen.FindGameAssetVersionParams{GameSourceID: o.GameSourceID, ExternalKey: externalKey,
		SourceFamily: o.Capability, SourceRole: a.Role, SourceIdentity: a.Identity, BindingID: o.BindingID,
		ObservationID: o.ObservationID, ScopeKey: o.ScopeKey, EgressRegion: o.EgressRegion})
}

func steamArtworkPath(v *SteamAsset, app string) bool {
	u, err := url.Parse(v.URL)
	if err != nil {
		return false
	}
	assetPath := u.EscapedPath()
	if v.Family == "community" {
		return u.Hostname() == "cdn.cloudflare.steamstatic.com" &&
			strings.HasPrefix(assetPath, "/steamcommunity/public/images/apps/"+app+"/")
	}
	switch u.Hostname() {
	case "shared.steamstatic.com", "shared.akamai.steamstatic.com", "shared.cloudflare.steamstatic.com",
		"shared.fastly.steamstatic.com", "shared.st.dl.eccdnx.com", "shared.cdn.steamchina.queniuam.com":
		return strings.HasPrefix(assetPath, "/store_item_assets/steam/apps/"+app+"/") ||
			strings.HasPrefix(assetPath, "/steam/apps/"+app+"/")
	}
	return false
}
