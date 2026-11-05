package wallpaper

import (
	"bytes"
	json "encoding/json/v2"
	"os"
	"testing"
)

func TestLegacyImportContract(t *testing.T) {
	data, e := os.ReadFile("../handlers/backgrounds.json")
	if e != nil {
		t.Fatal(e)
	}
	items, e := ReadLegacy(bytes.NewReader(data))
	if e != nil || len(items) != 1 {
		t.Fatal("real migration input invalid", e)
	}
	for _, change := range []func(*LegacyBackground){func(i *LegacyBackground) { i.SFW = false }, func(i *LegacyBackground) { i.Image.Key = "../unsafe" }, func(i *LegacyBackground) { i.Preview.ContentType = "image/avif" }, func(i *LegacyBackground) { i.Image.SHA256 = "invalid" }, func(i *LegacyBackground) { i.Image.Width = 0 }, func(i *LegacyBackground) { i.SourceURL = "https://user:credential@example.com/file" }} {
		copy := items[0]
		change(&copy)
		b, _ := json.Marshal([]LegacyBackground{copy})
		if _, e := ReadLegacy(bytes.NewReader(b)); e == nil {
			t.Fatal("invalid legacy record accepted")
		}
	}
	b, _ := json.Marshal([]LegacyBackground{items[0], items[0]})
	if _, e := ReadLegacy(bytes.NewReader(b)); e == nil {
		t.Fatal("duplicate originals accepted")
	}
}
func TestSourceMetadataRejectsCredentials(t *testing.T) {
	for _, source := range []string{"https://user:password@example.com/art", "https://example.com/art?token=private", "https://example.com/art?X-Amz-Credential=private", "https://example.com/art#access_token=private"} {
		m := Metadata{Title: "Artwork", SourceURL: source}
		if m.Validate() == nil {
			t.Fatal("credential-bearing public source accepted")
		}
	}
	m := Metadata{Title: "Artwork", SourceURL: "https://example.com/art?view=full"}
	if m.Validate() != nil {
		t.Fatal("normal source query refused")
	}
}
