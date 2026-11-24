package catalog

import (
	"net/mail"
	"net/url"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/gofurry/steam-go/addons/markup"
	"github.com/gofurry/steam-go/web/storefront"
	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

const SteamRichDataLimit = 384<<10 - 1024
const SteamRichEditBytes = 64 << 10

type RichRequirements struct {
	Minimum     *string `json:"minimum,omitzero"`
	Recommended *string `json:"recommended,omitzero"`
}

type RichSupport struct {
	URL   *string `json:"url,omitzero"`
	Email *string `json:"email,omitzero"`
}

type RichDescriptors struct {
	IDs   []int   `json:"ids"`
	Notes *string `json:"notes,omitzero"`
}

type RichRating struct {
	Rating      *string `json:"rating,omitzero"`
	RequiredAge *string `json:"required_age,omitzero"`
}

type RichPlatforms struct {
	Windows *bool `json:"windows,omitzero"`
	Mac     *bool `json:"mac,omitzero"`
	Linux   *bool `json:"linux,omitzero"`
}

type RichRelease struct {
	ComingSoon *bool  `json:"coming_soon,omitzero"`
	Date       string `json:"date_text,omitzero"`
}

// SteamRichContent carries the source fields in private candidate reads. In a
// reviewed snapshot the markup-bearing strings contain canonical safe HTML.
type SteamRichContent struct {
	Detailed    *string                      `json:"detailed_description,omitzero"`
	About       *string                      `json:"about_the_game,omitzero"`
	Short       *string                      `json:"short_description,omitzero"`
	Website     *string                      `json:"website,omitzero"`
	Support     *RichSupport                 `json:"support_info,omitzero"`
	PC          *RichRequirements            `json:"pc_requirements,omitzero"`
	Mac         *RichRequirements            `json:"mac_requirements,omitzero"`
	Linux       *RichRequirements            `json:"linux_requirements,omitzero"`
	Descriptors *RichDescriptors             `json:"content_descriptors,omitzero"`
	Ratings     map[string]RichRating        `json:"ratings,omitzero"`
	RequiredAge *int                         `json:"required_age,omitzero"`
	IsFree      *bool                        `json:"is_free,omitzero"`
	Platforms   *RichPlatforms               `json:"platforms,omitzero"`
	Controller  *string                      `json:"controller_support,omitzero"`
	Release     *RichRelease                 `json:"release,omitzero"`
	Languages   []storefront.LanguageSupport `json:"languages,omitzero"`
	Truncated   []string                     `json:"truncated,omitzero"`
	Invalid     []string                     `json:"invalid,omitzero"`
}

type SteamRichSource struct {
	SteamRichContent
	AppID              uint32  `json:"app_id"`
	Tier               string  `json:"tier"`
	URL                string  `json:"source_url"`
	SupportedLanguages *string `json:"supported_languages,omitzero"`
}

func SteamRichLink(raw string) string {
	if len(raw) > 2048 || strings.ContainsAny(raw, "\\") || strings.ContainsFunc(raw, func(r rune) bool {
		return unicode.IsControl(r) || unicode.IsSpace(r)
	}) {
		return ""
	}
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil ||
		u.Port() != "" && u.Port() != "443" || strings.HasSuffix(u.Host, ":") {
		return ""
	}
	return u.String()
}

func cleanRichURL(raw *string) *string {
	if raw == nil || *raw == "" {
		return raw
	}
	value := SteamRichLink(*raw)
	if value == "" {
		return nil
	}
	return &value
}

func richAttribute(node *html.Node, key string) string {
	for _, attr := range node.Attr {
		if attr.Key == key && attr.Namespace == "" {
			return attr.Val
		}
	}
	return ""
}

func richMarkup(value string) (string, bool) {
	if !strings.Contains(value, "<") && strings.Contains(value, "[") {
		value, _ = markup.CleanSteamContent(value, markup.WithVideoTags(false))
	}
	context := &html.Node{Type: html.ElementNode, Data: "div", DataAtom: atom.Div}
	nodes, err := html.ParseFragment(strings.NewReader(value), context)
	if err != nil {
		return "", false
	}
	var out strings.Builder
	overflow := false
	var walk func(*html.Node, int)
	walk = func(node *html.Node, depth int) {
		if out.Len() > SteamRichDataLimit || depth > 128 {
			overflow = true
			return
		}
		if node.Type == html.TextNode {
			_ = html.Render(&out, &html.Node{Type: html.TextNode, Data: node.Data})
			return
		}
		if node.Type != html.ElementNode || node.Namespace != "" {
			return
		}
		tag := node.Data
		switch tag {
		case "script", "style", "iframe", "video", "audio", "source", "object", "embed", "template":
			return
		case "img":
			if link := publicMediaURL(richAttribute(node, "src"), ""); link != "" {
				_ = html.Render(&out, &html.Node{Type: html.ElementNode, Data: "img", DataAtom: atom.Img,
					Attr: []html.Attribute{{Key: "src", Val: link}, {Key: "alt", Val: richAttribute(node, "alt")},
						{Key: "loading", Val: "lazy"}, {Key: "referrerpolicy", Val: "no-referrer"}}})
			}
			return
		case "br", "hr":
			out.WriteString("<" + tag + ">")
			return
		case "a":
			if link := SteamRichLink(richAttribute(node, "href")); link != "" {
				out.WriteString("<a href=\"" + html.EscapeString(link) + "\" rel=\"noopener noreferrer nofollow\" referrerpolicy=\"no-referrer\">")
			} else {
				tag = ""
			}
		case "p", "div", "ul", "ol", "li", "blockquote", "pre", "code", "strong", "em", "b", "i", "u", "s":
			out.WriteString("<" + tag + ">")
		case "h1", "h2", "h3", "h4", "h5", "h6":
			tag = "h4"
			out.WriteString("<h4>")
		default:
			tag = ""
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			walk(child, depth+1)
		}
		if tag != "" {
			out.WriteString("</" + tag + ">")
		}
	}
	for _, node := range nodes {
		walk(node, 0)
	}
	if overflow || out.Len() > SteamRichDataLimit {
		return "", true
	}
	return strings.TrimSpace(out.String()), false
}

// CleanSteamEventBody uses the same final allowlist as published rich details.
func CleanSteamEventBody(raw string) (string, string, string) {
	prepared, err := markup.CleanSteamContent(raw, markup.WithVideoTags(false))
	if err != nil {
		return "", "", ""
	}
	clean, overflow := richMarkup(prepared)
	if overflow || clean == "" || len(clean) > 256<<10 {
		return "", "", ""
	}
	plain := richTextSummary(clean)
	if len(plain) > 256<<10 {
		return "", "", ""
	}
	summary := []rune(strings.Join(strings.Fields(plain), " "))
	return clean, plain, strings.TrimSpace(string(summary[:min(180, len(summary))]))
}

func richTextSummary(clean string) string {
	text, _ := markup.PlainText(clean)
	return text
}

func reviewRichContent(raw SteamRichContent) SteamRichContent {
	v := raw
	clean := func(name string, field *string) *string {
		if field == nil {
			return nil
		}
		value, overflow := richMarkup(*field)
		if overflow {
			v.Truncated = append(slices.Clone(v.Truncated), name)
			return nil
		}
		return &value
	}
	requirements := func(name string, v *RichRequirements) *RichRequirements {
		if v == nil {
			return nil
		}
		return &RichRequirements{clean(name, v.Minimum), clean(name, v.Recommended)}
	}
	v.Detailed = clean("detailed_description", raw.Detailed)
	v.About = clean("about_the_game", raw.About)
	v.Short = clean("short_description", raw.Short)
	v.PC = requirements("pc_requirements", raw.PC)
	v.Mac = requirements("mac_requirements", raw.Mac)
	v.Linux = requirements("linux_requirements", raw.Linux)
	v.Website = cleanRichURL(raw.Website)
	if raw.Support != nil {
		v.Support = &RichSupport{URL: cleanRichURL(raw.Support.URL)}
		if email := raw.Support.Email; email != nil && (len(*email) <= 254) {
			if a, err := mail.ParseAddress(*email); *email == "" || err == nil && a.Address == *email {
				v.Support.Email = email
			}
		}
	}
	if raw.Descriptors != nil {
		v.Descriptors = &RichDescriptors{IDs: raw.Descriptors.IDs, Notes: clean("content_descriptors", raw.Descriptors.Notes)}
	}
	if len(v.Truncated) > len(raw.Truncated) {
		slices.Sort(v.Truncated)
		v.Truncated = slices.Compact(v.Truncated)
	}
	return v
}

func safeEditorialText(text string) bool {
	return len(text) <= SteamRichEditBytes && utf8.ValidString(text) && !strings.ContainsFunc(text, func(r rune) bool {
		return unicode.IsControl(r) && r != '\n' && r != '\r' && r != '\t'
	})
}

func richEditorialHTML(text string) *string {
	value := strings.ReplaceAll(text, "\r\n", "\n")
	lines := strings.Split(value, "\n\n")
	for i, line := range lines {
		lines[i] = "<p>" + strings.ReplaceAll(html.EscapeString(line), "\n", "<br>") + "</p>"
	}
	value = strings.Join(lines, "")
	return &value
}

func richHasContent(v SteamRichContent) bool {
	return v.Detailed != nil || v.About != nil || v.Short != nil || v.PC != nil || v.Mac != nil || v.Linux != nil ||
		v.Website != nil || v.Support != nil || v.Descriptors != nil || len(v.Languages) != 0 || len(v.Ratings) != 0 ||
		v.RequiredAge != nil || v.Platforms != nil || v.Controller != nil || v.Release != nil || v.IsFree != nil
}

func (v SteamRichSource) ValidDiagnostics() bool {
	return validRichDiagnostics(v.Invalid) && validRichDiagnostics(v.Truncated) &&
		!slices.ContainsFunc(v.Invalid, func(field string) bool { return slices.Contains(v.Truncated, field) })
}

func validRichDiagnostics(values []string) bool {
	fields := []string{"about_the_game", "content_descriptors", "controller_support", "detailed_description", "is_free", "linux_requirements",
		"mac_requirements", "pc_requirements", "platforms", "ratings", "release_date", "required_age", "short_description",
		"support_info", "supported_languages", "website"}
	return len(values) <= len(fields) && slices.IsSorted(values) && len(slices.Compact(slices.Clone(values))) == len(values) &&
		!slices.ContainsFunc(values, func(value string) bool { return !slices.Contains(fields, value) })
}
