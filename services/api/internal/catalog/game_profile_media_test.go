package catalog

import (
	json "encoding/json/v2"
	"slices"
	"strconv"
	"strings"
	"testing"
	"uuid"
)

func TestPublicMediaURL(t *testing.T) {
	const cdn = "https://shared.steamstatic.com"
	const streaming = "https://video.akamai.steamstatic.com"
	long := cdn + "/" + strings.Repeat("a", publicMediaURLBytes-len(cdn)-len("/.png")) + ".png"
	for _, tc := range []struct {
		name, url, format string
		valid             bool
	}{
		{"image", cdn + "/store_item_assets/steam/apps/42/library%20hero.png", "", true},
		{"regional mirror", "https://shared.st.dl.eccdnx.com/steam/apps/42/library_hero.jpg", "", true},
		{"China mirror", "https://shared.cdn.steamchina.queniuam.com/store_item_assets/steam/apps/42/logo.png", "", true},
		{"mirror video", "https://shared.st.dl.eccdnx.com/steam/apps/42/movie.mp4", ".mp4", false},
		{"mirror wrong path", "https://shared.st.dl.eccdnx.com/unrelated/logo.png", "", false},
		{"legacy Steam", "https://steamcdn-a.akamaihd.net/steam/apps/42/header.jpg", "", true},
		{"community", "https://cdn.cloudflare.steamstatic.com/steamcommunity/public/images/apps/42/icon.jpg", "", true},
		{"default port", cdn + ":443/image.webp", "", true},
		{"MP4", streaming + "/movie.MP4", ".mp4", true},
		{"WebM", streaming + "/movie.webm", ".webm", true},
		{"HLS", streaming + "/hls.m3u8", ".m3u8", true},
		{"DASH", streaming + "/dash.mpd", ".mpd", true},
		{"exact byte limit", long, "", true},
		{"over byte limit", long[:len(long)-4] + "x.png", "", false},
		{"external", "https://example.com/image.jpg", "", false},
		{"suffix spoof", "https://shared.steamstatic.com.example.com/image.jpg", "", false},
		{"missing boundary", "https://notsteamstatic.com/image.jpg", "", false},
		{"mirror spoof", "https://shared.st.dl.eccdnx.com.example.com/steam/apps/42/logo.png", "", false},
		{"userinfo", "https://user:pass@shared.steamstatic.com/image.jpg", "", false},
		{"fragment", cdn + "/image.jpg#ref", "", false},
		{"query", cdn + "/image.jpg?token=private", "", false},
		{"empty query", cdn + "/image.jpg?", "", false},
		{"port", cdn + ":444/image.jpg", "", false},
		{"empty port", cdn + ":/image.jpg", "", false},
		{"insecure transport", "http://shared.steamstatic.com/image.jpg", "", false},
		{"traversal", cdn + "/a/../image.jpg", "", false},
		{"encoded traversal", cdn + "/a/%2e%2e/image.jpg", "", false},
		{"mixed slashes", cdn + "/a%2fb/image.jpg", "", false},
		{"double encoding", cdn + "/a/%252e%252e/image.jpg", "", false},
		{"backslash", cdn + "/a%5cb/image.jpg", "", false},
		{"invalid encoding", cdn + "/a%qq.jpg", "", false},
		{"non-UTF8", cdn + "/a%ff.jpg", "", false},
		{"control", cdn + "/a%0a.jpg", "", false},
		{"format control", cdn + "/a%E2%80%AE.jpg", "", false},
		{"SVG", cdn + "/asset.svg", "", false},
		{"wrong direct format", streaming + "/movie.webm", ".mp4", false},
		{"wrong manifest", streaming + "/dash.mpd", ".m3u8", false},
		{"thumbnail as video", streaming + "/movie.jpg", ".mp4", false},
		{"extensionless video", streaming + "/movie", ".mp4", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			want := ""
			if tc.valid {
				want = tc.url
			}
			if got := publicMediaURL(tc.url, tc.format); got != want {
				t.Fatalf("unexpected URL validation result (accepted=%t, want accepted=%t)", got != "", tc.valid)
			}
		})
	}
}

func TestPublishedProfileMedia(t *testing.T) {
	const img = "https://shared.steamstatic.com/steam/apps/42/"
	const video = "https://video.akamai.steamstatic.com/steam/apps/42/"
	const private = "internal-projection-example"
	origin := func() GameMediaOrigin {
		return GameMediaOrigin{GameSourceID: uuid.NewV7(), ObservationID: uuid.NewV7(), Namespace: private, ContentHash: private}
	}
	highlight := false
	cover := &GameSelectedImage{Origin: origin(), Type: "library_capsule", Family: "library", ID: private, URL: img + "cover.jpg"}
	sources := SteamMovieSources{
		MP4480: video + "480.mp4", MP4Max: video + "max.mp4",
		WebM480: video + "480.webm", WebMMax: video + "max.webm",
		HLSH264: video + "hls.m3u8", DASHAV1: video + "av1.mpd", DASHH264: video + "h264.mpd",
	}
	media := &GameMediaSelectionV2{
		Version: 2, SelectionRevision: 99, Cover: cover,
		Images: []GameSelectedImage{
			{Origin: origin(), Type: "library_hero", Family: "library", URL: img + "hero.png"},
			{Origin: origin(), Type: "library_hero", Family: "library", URL: img + "duplicate.png"},
			{Origin: origin(), Type: "library_hero_2x", Family: "library", URL: img + "hero.png"},
			{Origin: origin(), Type: "library_logo", Family: "library", URL: img + "logo.png"},
			{Origin: origin(), Type: "header", Family: "store", URL: img + "header.jpg"},
			{Origin: origin(), Type: "main_capsule", Family: "store", URL: img + "capsule.jpg"},
			{Origin: origin(), Type: "bad-role", Family: private, URL: img + "invalid.png"},
		},
		Screenshots: []GameSelectedScreenshot{
			{Origin: origin(), SteamScreenshot: SteamScreenshot{ID: "4", Thumbnail: img + "t.jpg", Full: img + "f.jpg", Order: 1}},
			{Origin: origin(), SteamScreenshot: SteamScreenshot{ID: "0", Full: img + "z.jpg?#private", Thumbnail: img + "zz.jpg", Order: 0}},
		},
		Movies: []GameSelectedMovie{
			{Origin: origin(), SteamMovie: SteamMovie{ID: "9007199254740993", Name: "Trailer", Thumbnail: img + "poster.jpg", Highlight: &highlight, Order: 9, Sources: sources}},
			{Origin: origin(), SteamMovie: SteamMovie{ID: "7", Order: 0, Sources: SteamMovieSources{
				MP4Max: video + "secret.mp4?token=private", WebM480: video + "direct.mp4", HLSH264: video + "hls.m3u8", DASHH264: video + "h264.mpd",
			}}},
		},
	}

	locales := []PublishedGameMedia{{Locale: "en", MediaV2: media}, {Locale: "zh", Media: SteamMedia{Header: img + "legacy.jpg"}}}
	out := publishedProfileMedia(locales, "zh")
	if out == nil || !out.Fallback || out.Locale != "en" || out.Version != 2 || out.Cover == nil || out.Cover.Type != "library_capsule" {
		t.Fatalf("unexpected fallback or cover: %+v", out)
	}
	var roles []string
	for _, v := range out.Images {
		roles = append(roles, v.Type)
	}
	if !slices.Equal(roles, []string{"library_hero", "library_hero_2x", "library_logo", "header", "main_capsule"}) ||
		out.Images[0].URL != out.Images[1].URL {
		t.Fatal("distinct image roles, adoption order or role bounds were lost")
	}
	if len(out.Screenshots) != 2 || out.Screenshots[0] != (PublicGameScreenshot{ID: "4", Thumbnail: img + "t.jpg", Full: img + "f.jpg"}) ||
		out.Screenshots[1] != (PublicGameScreenshot{ID: "0", Thumbnail: img + "zz.jpg"}) {
		t.Fatal("screenshot identity, thumbnail, full image or adoption order was lost")
	}
	m := out.Movies[0]
	if len(out.Movies) != 2 || m.ID != "9007199254740993" || m.Name != "Trailer" || m.Highlight == nil || *m.Highlight ||
		m.Thumbnail != img+"poster.jpg" || m.Sources != sources || out.Movies[1].ID != "7" ||
		out.Movies[1].Sources != (SteamMovieSources{HLSH264: video + "hls.m3u8", DASHH264: video + "h264.mpd"}) {
		t.Fatal("movie metadata, formats, adoption order or URL filtering changed")
	}
	if media.Movies[1].Sources.MP4Max != video+"secret.mp4?token=private" {
		t.Fatal("projection modified the input snapshot")
	}
	raw, err := json.Marshal(out)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{private, cover.Origin.GameSourceID.String(), cover.Origin.ObservationID.String(), "\"origin\"", "\"selection_revision\"", "token=", "\"order\""} {
		if strings.Contains(string(raw), forbidden) {
			t.Fatalf("public media contains a private field or value")
		}
	}
	if !strings.Contains(string(raw), "\"movie_id\":\"9007199254740993\"") || !strings.Contains(string(raw), "\"highlight\":false") {
		t.Fatal("JSON lost the string ID or explicit false Highlight")
	}

	locales[1].MediaV2 = &GameMediaSelectionV2{Version: 2, Screenshots: media.Screenshots[:1]}
	zh := publishedProfileMedia(locales, "zh")
	en := publishedProfileMedia(locales, "en")
	if zh == nil || zh.Locale != "zh" || zh.Fallback || len(zh.Movies) != 0 || zh.Cover != nil ||
		en == nil || en.Locale != "en" || en.Fallback || len(en.Movies) != 2 {
		t.Fatal("preferred locale changed or merged another locale")
	}
	locales[0].MediaV2 = nil
	if out := publishedProfileMedia(locales, "en"); out == nil || !out.Fallback || out.Locale != "zh" {
		t.Fatal("English request did not fall back to published Chinese media")
	}
	locales[1].MediaV2 = nil
	noV2 := GameProfile{PublishedMedia: publishedProfileMedia(locales, "en")}
	legacy, err := json.Marshal(noV2)
	if err != nil || noV2.PublishedMedia != nil || strings.Contains(string(legacy), "\"published_media\"") {
		t.Fatal("profile with only legacy media acquired a v2 field")
	}
}

func TestPublishedProfileMediaBounds(t *testing.T) {
	long := func(ext string) string {
		base := "https://video.akamai.steamstatic.com/steam/apps/42/"
		return base + strings.Repeat("a", publicMediaURLBytes-len(base)-len(ext)) + ext
	}
	sources := SteamMovieSources{MP4480: long(".mp4"), MP4Max: long(".mp4"), WebM480: long(".webm"), WebMMax: long(".webm"),
		HLSH264: long(".m3u8"), DASHH264: long(".mpd"), DASHAV1: long(".mpd")}
	m := GameMediaSelectionV2{Version: 2}
	for i := range 10 {
		id := strconv.Itoa(i + 1)
		m.Screenshots = append(m.Screenshots, GameSelectedScreenshot{SteamScreenshot: SteamScreenshot{ID: id, Thumbnail: long(".jpg"), Full: long(".jpg")}})
	}
	for i := range 5 {
		id := strconv.Itoa(i + 1)
		m.Movies = append(m.Movies, GameSelectedMovie{SteamMovie: SteamMovie{ID: id, Thumbnail: long(".png"), Name: strings.Repeat("n", 256), Sources: sources}})
	}
	out := projectPublishedMedia(&m, "en", false)
	if out == nil || len(out.Screenshots) != 8 || out.Screenshots[7].ID != "8" || len(out.Movies) != 4 || out.Movies[3].ID != "4" {
		t.Fatal("public count or order bounds not enforced")
	}
	for _, movie := range out.Movies {
		if movie.Sources != sources {
			t.Fatal("long URLs caused loss of a valid format")
		}
	}
	raw, err := json.Marshal(out)
	if err != nil || len(raw) < 96<<10 || len(raw) > 128<<10 {
		t.Fatalf("unexpected near-limit fixture size %d: %v", len(raw), err)
	}
	t.Logf("near-limit published_media JSON: %d bytes", len(raw))
}
