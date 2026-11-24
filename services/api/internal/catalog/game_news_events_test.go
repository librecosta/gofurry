package catalog

import (
	"strings"
	"testing"
)

func TestCleanSteamEventBody(t *testing.T) {
	body := `[h2]Update[/h2][quote]From the team[/quote]
[list][*]First[/*][*]Second[/*][/list]
[img]https://shared.steamstatic.com/store_item_assets/steam/apps/730/header.jpg[/img]
[video]https://cdn.cloudflare.steamstatic.com/test.mp4[/video]
<a href="https://example.org/a" onclick="alert(1)">Official notes</a>
<img src="https://outside.example/unsafe.png" onerror="alert(1)">
<a href="javascript:alert(1)">Broken link</a>
<iframe src="https://outside.example/frame"></iframe><script>alert(1)</script>`
	clean, plain, summary := CleanSteamEventBody(body)
	for _, required := range []string{"<h4>Update</h4>", "<blockquote>", "<li>Second</li>", "<img", "loading=\"lazy\"",
		"referrerpolicy=\"no-referrer\"", "<a href=\"https://example.org/a\""} {
		if !strings.Contains(clean, required) {
			t.Errorf("sanitizer lost %q in %q", required, clean)
		}
	}
	for _, forbidden := range []string{"<script", "<iframe", "<video", "javascript:", "outside.example", "onclick", "onerror", "alert("} {
		if strings.Contains(clean, forbidden) {
			t.Errorf("sanitizer retained %q", forbidden)
		}
	}
	if !strings.Contains(plain, "From the team") || len(summary) > 180 {
		t.Fatalf("bad plaintext or summary")
	}
}
