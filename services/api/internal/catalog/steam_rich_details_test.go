package catalog

import (
	"strings"
	"testing"
)

func TestRichDetailsCanonicalMarkup(t *testing.T) {
	raw := `<h2>About</h2><p>Words <strong>bold</strong></p><ul><li>One<li>Two</ul>
		<script>alert(1)</script><iframe src="https://example.org"></iframe>
		<svg><a href="javascript:alert(1)">x</a></svg>
		<a href="https://example.org/path" onclick="alert(2)">official</a>
		<a href="javascr&#105;pt:alert(3)">invalid</a>
		<img src="https://shared.steamstatic.com/abc.png" onerror="alert(4)">
		<img src="https://shared.steamstatic.com.evil.test/tracker.png">
		<video controls autoplay src="https://example.org/video"></video>`
	got, overflow := richMarkup(raw)
	if overflow {
		t.Fatal("unexpected markup limit")
	}
	for _, expected := range []string{"<h4>About</h4>", "<strong>bold</strong>", "<li>One</li><li>Two</li>", "https://example.org/path",
		"noopener noreferrer nofollow", "referrerpolicy", "https://shared.steamstatic.com/abc.png"} {
		if !strings.Contains(got, expected) {
			t.Errorf("missing %q in %q", expected, got)
		}
	}
	for _, banned := range []string{"script", "alert(", "onerror", "<video", "iframe", "evil.test", "<svg"} {
		if strings.Contains(got, banned) {
			t.Errorf("unsafe markup contains %s", banned)
		}
	}
	if v, overflow := richMarkup("[b]Steam[/b]"); overflow || !strings.Contains(v, "<b>Steam</b>") && !strings.Contains(v, "<strong>Steam</strong>") {
		t.Fatalf("Steam BBCode missing: %q", v)
	}
	const editorial = "<img src=x>\nplain"
	if got := *richEditorialHTML(editorial); !strings.Contains(got, "&lt;img src=x&gt;<br>") {
		t.Fatalf("editorial text was treated as markup: %q", got)
	}
}

func TestRichDetailsMarkupBudgetAndSourceLanguage(t *testing.T) {
	long := strings.Repeat("<div>", 130) + "nested" + strings.Repeat("</div>", 130)
	review := reviewRichContent(SteamRichContent{Detailed: &long})
	if review.Detailed != nil || len(review.Truncated) != 1 || review.Truncated[0] != "detailed_description" {
		t.Fatal("oversized review did not diagnose the complete field")
	}
	en, zh := "<p>English</p>", "<p>中文</p>"
	if !sameRichSourceText(SteamRichContent{Detailed: &en}, SteamRichContent{Detailed: &en}) ||
		sameRichSourceText(SteamRichContent{Detailed: &zh}, SteamRichContent{Detailed: &en}) ||
		sameRichSourceText(SteamRichContent{}, SteamRichContent{}) {
		t.Fatal("localized prose comparison inferred an unsupported language")
	}
}

func TestPublishedRichDetailsLocale(t *testing.T) {
	text := "<p>test</p>"
	rows := []PublishedGameMedia{{Locale: "en", RichDetails: &PublicGameRichDetails{SteamRichContent: SteamRichContent{
		Detailed: &text, Truncated: []string{"about_the_game"}}}}}
	v := publishedGameRichDetails(rows, "zh")
	if v == nil || !v.Fallback || v.Locale != "en" || v.Detailed == nil || len(v.Truncated) != 0 {
		t.Fatalf("unexpected public fallback: %+v", v)
	}
}
