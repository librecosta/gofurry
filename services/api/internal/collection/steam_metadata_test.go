package collection

import (
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"strings"
	"testing"
)

func TestSteamMetadataNormalization(t *testing.T) {
	const source = `"app_id":42,"tier":"observed_storefront","source_url":"https://store.steampowered.com/api/appdetails"`
	o := observation{identity: identity{Capability: "steam.details"}, State: "present",
		Scope: segmentScope{scope: scope{Locale: "zh-cn", Region: "cn"}, Dimensions: map[string]string{"section": "details"}}}
	o.Data = jsontext.Value(`{` + source + `,"name":"森林传说","description":"狐狸打开图书馆。","supported_languages":"English\u0007中文","developers":["Studio","Studio","<script>private</script>"],"publishers":["Press"],"platforms":{"windows":true,"mac":false},"release":{"date_text":"Q3 2027"},"genres":[{"id":"2","name":"Indie"},{"id":"01","name":"invalid"},{"id":"2","name":"Indie"}],"unknown_private":"do-not-store"}`)
	c, err := normalizeSteam(o, "42")
	if err != nil {
		t.Fatal(err)
	}
	var d steamDetail
	if err = json.Unmarshal(c.Data, &d); err != nil {
		t.Fatal(err)
	}
	if !d.Localized || len(d.Developers) != 1 || d.SupportedLanguages != "English 中文" ||
		d.Platforms == nil || d.Platforms.Windows == nil || !*d.Platforms.Windows || d.Platforms.Mac == nil || *d.Platforms.Mac || d.Platforms.Linux != nil ||
		d.Release == nil || d.Release.ComingSoon != nil || d.Release.DateText != "Q3 2027" || len(d.Genres) != 1 || strings.Contains(string(c.Data), "private") {
		t.Fatalf("unexpected normalization: %s", c.Data)
	}
	o.Data = jsontext.Value(`{` + source + `,"name":"Older payload","description":"English fallback"}`)
	c, err = normalizeSteam(o, "42")
	if err != nil || strings.Contains(string(c.Data), "localized") {
		t.Fatalf("legacy/English fallback: %s %v", c.Data, err)
	}
	o.Scope.Region = "invalid"
	if _, err = normalizeSteam(o, "42"); err == nil {
		t.Fatal("unknown store country accepted")
	}
	o.Scope.Region = "hk"
	o.Scope.Dimensions["section"] = "media"
	o.Data = jsontext.Value(`{` + source + `,"header":"https://user@cdn.steamstatic.com/invalid","capsule":"http://cdn.steamstatic.com/invalid","screenshots":["https://cdn.steamstatic.com/shot.jpg?token=discard","https://cdn.steamstatic.com/shot.jpg","https://cdn.steamstatic.com.evil.example/invalid","https://cdn.steamstatic.com:443/invalid","https://cdn.steamstatic.com/a%5cb","https://cdn.steamstatic.com/1","https://cdn.steamstatic.com/2","https://cdn.steamstatic.com/3","https://cdn.steamstatic.com/over"],"movies":["https://cdn.steamcontent.com/1.mp4","https://cdn.steamcontent.com/2.mp4","https://cdn.steamcontent.com/3.mp4","https://steamcdn-a.akamaihd.net/4.mp4","https://cdn.steamcontent.com/over"]}`)
	c, err = normalizeSteam(o, "42")
	var media steamMedia
	if err != nil || json.Unmarshal(c.Data, &media) != nil || media.Header != "" || media.Capsule != "" || len(media.Movies) != 4 ||
		len(media.Screenshots) != 4 || strings.Contains(string(c.Data), "invalid") || strings.Contains(string(c.Data), "discard") || strings.Contains(string(c.Data), "over") {
		t.Fatalf("unsafe or unbounded media: %s %v", c.Data, err)
	}
	if _, err = normalizeSteam(o, "43"); err == nil {
		t.Fatal("foreign AppID accepted")
	}
}
