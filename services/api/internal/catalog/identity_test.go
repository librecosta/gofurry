package catalog

import (
	"strings"
	"testing"
	"uuid"
)

func TestIndependentIdentifiers(t *testing.T) {
	const count = 12000
	ids, public := map[uuid.UUID]bool{}, map[string]bool{}
	frequency := map[byte]int{}
	increasing := 0
	previous := ""
	for range count {
		id, token := uuid.NewV7(), newPublicID()
		if id.String()[14] != '7' || ids[id] || !ValidPublicID(token) || public[token] {
			t.Fatal("unexpected UUIDv7 or Public ID")
		}
		bytes, err := publicEncoding.DecodeString(token)
		if err != nil || len(bytes) != 10 {
			t.Fatal("Public ID is not a canonical 80-bit encoding")
		}
		if previous < token {
			increasing++
		}
		frequency[token[0]]++
		ids[id], public[token] = true, true
		previous = token
	}
	if increasing < count*4/10 || increasing > count*6/10 || len(frequency) != 32 {
		t.Fatal("Public IDs exhibit chronological ordering or missing symbols")
	}
	for _, n := range frequency {
		if n < 220 || n > 550 {
			t.Fatal("Public ID symbol frequency is outside the test bounds")
		}
	}
	for _, id := range []string{"0123456789ABCDEF", "abc", strings.Repeat("i", 16), strings.Repeat("0", 17)} {
		if ValidPublicID(id) {
			t.Fatal("invalid Public ID")
		}
	}
}

func TestEditorialValidation(t *testing.T) {
	english := Localization{Locale: "en", Name: "Example _ Community"}
	chinese := Localization{Locale: "zh", Name: "示例网站"}
	for _, tc := range []struct {
		in             Editorial
		previous, want string
	}{
		{Editorial{Localizations: []Localization{chinese, english}}, "", "example-community"},
		{Editorial{Localizations: []Localization{english}}, "previous-slug", "previous-slug"},
		{Editorial{Slug: " Edited-Slug ", Localizations: []Localization{chinese}}, "", "edited-slug"},
		{Editorial{Localizations: []Localization{chinese}}, "", ""},
		{Editorial{Slug: "../example"}, "", ""},
		{Editorial{Slug: "a--b"}, "", ""},
		{Editorial{Slug: "name", Localizations: []Localization{english, english}}, "", ""},
		{Editorial{Slug: "name", Localizations: []Localization{{Locale: "en", Name: "line\nbreak"}}}, "", ""},
	} {
		out, err := normalizeEditorial(tc.in, tc.previous)
		if (err != nil) != (tc.want == "") || out.Slug != tc.want {
			t.Fatalf("unexpected slug %q or error %v", out.Slug, err)
		}
	}
}

func TestNavigationURLsAndSources(t *testing.T) {
	for raw, want := range map[string]string{
		"HTTPS://BÜCHER.DE:0443/A%2Fb?q=one&q=two&last=": "https://xn--bcher-kva.de/A%2Fb?q=one&q=two&last=",
		"http://EXAMPLE.com.:80/path?q=A+B":              "http://example.com/path?q=A+B",
		"https://[2001:DB8::1]:444/":                     "https://[2001:db8::1]:444/",
		"https://example.com/?":                          "https://example.com/?",
	} {
		got, err := normalizeURL(raw)
		if got != want || err != nil {
			t.Fatalf("normalize %q returned %q, %v", raw, got, err)
		}
	}
	for _, raw := range []string{"javascript:alert(1)", "//example.com", "https://x:y@example.com/", "https://example.com/#",
		"https://example.com:65536/", "https://example.com/a%00", "https://example.com/?q=%0A", "https://example.com/?%61ccess_token=secret",
		"https://example.com/?path=x;access_token=secret", "https://example.com/a\\b", "https://[fe80::1%25zone]/"} {
		if _, err := normalizeURL(raw); err == nil {
			t.Fatalf("accepted invalid URL %q", raw)
		}
	}
	for _, app := range []string{"", "0", "4294967296", "+1", "-1", "123.4", "١"} {
		if _, err := normalizeSources([]Source{{PlatformCode: "steam", ExternalKey: app}}); err == nil {
			t.Fatalf("accepted invalid AppID %q", app)
		}
	}
	got, err := normalizeSources([]Source{{PlatformCode: "Steam", ExternalKey: "0004294967295"}, {PlatformCode: "itch_io", ExternalKey: "maker/game"}})
	if err != nil || len(got) != 2 || got[1].ExternalKey != "4294967295" || got[1].SourceURL != "https://store.steampowered.com/app/4294967295/" {
		t.Fatalf("invalid normalized Steam and manual source result %v %v", got, err)
	}
	if _, err := normalizeSources([]Source{{PlatformCode: "steam", ExternalKey: "42", SourceURL: "https://example.com/app/42"}}); err == nil {
		t.Fatal("accepted a conflicting Steam source link")
	}
}
