package collection

import (
	"cmp"
	json "encoding/json/v2"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"unicode"

	"github.com/librecosta/gofurry/services/api/internal/catalog"
)

const steamProjectionRules = "gofurry-steam-1"

type steamDetail struct {
	steamProvenance
	catalog.SteamDetails
	Localized bool `json:"localized,omitzero"`
}

type steamMedia struct {
	steamProvenance
	catalog.SteamMedia
}

func steamScopeOK(o observation) bool {
	switch o.Capability {
	case "steam.details":
		return slices.Contains([]string{"details", "price", "media"}, o.Scope.Dimensions["section"]) &&
			(o.Scope.Locale == "en" || o.Scope.Locale == "zh-cn") &&
			slices.Contains([]string{"cn", "us", "hk"}, o.Scope.Region)
	case "steam.news", "steam.players":
		return o.Scope.Dimensions["section"] == strings.TrimPrefix(o.Capability, "steam.") &&
			o.Scope.Locale == "" && o.Scope.Region == ""
	default:
		return false
	}
}

func steamPlainText(raw string, limit int) string {
	// Harvest already removes markup and script contents. Drop a field with
	// residual markup before any presentation or future rendering.
	if strings.ContainsAny(raw, "<>") {
		return ""
	}
	return clean(raw, limit)
}

func steamNames(values []string) []string {
	names := make([]string, 0, min(len(values), 12))
	for _, value := range values[:min(len(values), 12)] {
		if name := steamPlainText(value, 128); name != "" {
			names = append(names, name)
		}
	}
	slices.Sort(names)
	return slices.Compact(names)
}

func steamLabels(values []catalog.SteamLabel) []catalog.SteamLabel {
	labels := make([]catalog.SteamLabel, 0, min(len(values), 24))
	for _, value := range values[:min(len(values), 24)] {
		id, err := strconv.ParseUint(value.ID, 10, 64)
		if err == nil && id > 0 && strconv.FormatUint(id, 10) == value.ID {
			labels = append(labels, catalog.SteamLabel{ID: value.ID, Name: steamPlainText(value.Name, 128)})
		}
	}
	slices.SortFunc(labels, func(a, b catalog.SteamLabel) int {
		return cmp.Or(strings.Compare(a.ID, b.ID), strings.Compare(a.Name, b.Name))
	})
	return slices.CompactFunc(labels, func(a, b catalog.SteamLabel) bool { return a.ID == b.ID })
}

func normalizedSteamDetails(o observation) (steamDetail, error) {
	var v steamDetail
	if json.Unmarshal(o.Data, &v) != nil {
		return v, errProtocol
	}
	v.Name = steamPlainText(v.Name, 256)
	if v.Name == "" {
		return v, errProtocol
	}
	v.Type = steamPlainText(v.Type, 48)
	v.Description = steamPlainText(v.Description, 1536)
	v.SupportedLanguages = steamPlainText(v.SupportedLanguages, 768)
	v.Developers, v.Publishers = steamNames(v.Developers), steamNames(v.Publishers)
	v.Categories, v.Genres = steamLabels(v.Categories), steamLabels(v.Genres)
	if v.Release != nil {
		v.Release.DateText = steamPlainText(v.Release.DateText, 128)
	}
	// A requested Chinese locale may come back with an English fallback.
	// Require Chinese-script synopsis text before promoting a zh presentation.
	v.Localized = o.Scope.Locale == "zh-cn" && hasChineseSynopsis(v.Description)
	return v, nil
}

func hasChineseSynopsis(text string) bool {
	han, letters := 0, 0
	for _, r := range text {
		switch {
		case unicode.Is(unicode.Hiragana, r) || unicode.Is(unicode.Katakana, r) || unicode.Is(unicode.Hangul, r):
			return false
		case unicode.Is(unicode.Han, r):
			han++
		case unicode.IsLetter(r):
			letters++
		}
	}
	return han >= 2 && han >= letters
}

func steamAssetURL(raw string) string {
	if len(raw) > 512 || strings.ContainsAny(raw, "<>") || strings.ContainsFunc(raw, unicode.IsSpace) {
		return ""
	}
	link := publicURL(raw)
	u, err := url.Parse(link)
	if err != nil || u.Scheme != "https" || u.Host != u.Hostname() || u.Path == "" || u.Path == "/" ||
		strings.ContainsAny(u.Path, "\\<>") || strings.ContainsFunc(u.Path, unicode.IsControl) {
		return ""
	}
	host := u.Hostname()
	if strings.HasSuffix(host, ".steamstatic.com") || strings.HasSuffix(host, ".steamcontent.com") ||
		host == "steamcdn-a.akamaihd.net" || host == "steamuserimages-a.akamaihd.net" {
		return link
	}
	return ""
}

func steamAssets(in []string, limit int) []string {
	out := make([]string, 0, min(len(in), limit))
	for _, raw := range in[:min(len(in), limit)] {
		if link := steamAssetURL(raw); link != "" {
			out = append(out, link)
		}
	}
	slices.Sort(out)
	return slices.Compact(out)
}

func normalizedSteamMedia(o observation) (steamMedia, error) {
	var v steamMedia
	if json.Unmarshal(o.Data, &v) != nil {
		return v, errProtocol
	}
	v.Header, v.Capsule = steamAssetURL(v.Header), steamAssetURL(v.Capsule)
	v.Screenshots, v.Movies = steamAssets(v.Screenshots, 8), steamAssets(v.Movies, 4)
	if v.Header == "" && v.Capsule == "" && len(v.Screenshots) == 0 && len(v.Movies) == 0 {
		return v, errProtocol
	}
	return v, nil
}
