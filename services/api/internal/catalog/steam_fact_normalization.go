package catalog

import (
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"slices"
	"strings"

	"github.com/gofurry/steam-go/web/storefront"
)

type SteamReleaseFact struct {
	State       string `json:"state"`
	ComingSoon  *bool  `json:"coming_soon,omitzero"`
	RawText     string `json:"raw_text"`
	Precision   string `json:"precision"`
	ExactDate   string `json:"exact_date,omitempty"`
	Year        int    `json:"year,omitzero"`
	Month       int    `json:"month,omitzero"`
	Quarter     int    `json:"quarter,omitzero"`
	WindowStart string `json:"window_start,omitempty"`
	WindowEnd   string `json:"window_end,omitempty"`
}

type SteamLanguageFact struct {
	storefront.LanguageSupport
	SortOrder int `json:"sort_order"`
}

type SteamLanguagesFact struct {
	Raw   string              `json:"raw"`
	Items []SteamLanguageFact `json:"items"`
}

type SteamFactValue struct {
	Kind  string
	Value jsontext.Value
}

// SteamFactValues reduces an already identity-checked v3 observation. Each
// value has its own provenance so missing fields cannot replace valid ones.
func SteamFactValues(v SteamRichSource) ([]SteamFactValue, bool, error) {
	var facts []SteamFactValue
	var encodeErr error
	add := func(kind string, value any) {
		body, err := json.Marshal(value, json.Deterministic(true))
		if err != nil {
			encodeErr = err
			return
		}
		facts = append(facts, SteamFactValue{kind, body})
	}
	available := false
	if v.Release != nil {
		release := steamReleaseFact(v.Release)
		available = release.State == "available"
		if release.State != "unknown" {
			add("release", release)
		}
	}
	for _, field := range []struct {
		kind  string
		value *bool
	}{
		{"free", v.IsFree}, {"windows", platformFlag(v.Platforms, "windows")},
		{"mac", platformFlag(v.Platforms, "mac")}, {"linux", platformFlag(v.Platforms, "linux")},
	} {
		if field.value != nil {
			add(field.kind, *field.value)
		}
	}
	if v.SupportedLanguages != nil && strings.TrimSpace(*v.SupportedLanguages) != "" && len(v.Languages) > 0 &&
		slices.ContainsFunc(v.Languages, func(lang storefront.LanguageSupport) bool { return lang.Known }) {
		value := SteamLanguagesFact{Raw: *v.SupportedLanguages, Items: make([]SteamLanguageFact, 0, len(v.Languages))}
		for i, lang := range v.Languages {
			value.Items = append(value.Items, SteamLanguageFact{lang, i})
		}
		body, err := json.Marshal(value, json.Deterministic(true))
		if err != nil {
			encodeErr = err
		} else if len(body) <= 64<<10 {
			facts = append(facts, SteamFactValue{"languages", body})
		}
	}
	return facts, available, encodeErr
}

func platformFlag(p *RichPlatforms, platform string) *bool {
	if p == nil {
		return nil
	}
	switch platform {
	case "windows":
		return p.Windows
	case "mac":
		return p.Mac
	default:
		return p.Linux
	}
}

func steamReleaseFact(v *RichRelease) SteamReleaseFact {
	coming := v.ComingSoon != nil && *v.ComingSoon
	n := storefront.NormalizeReleaseDate(&storefront.StoreReleaseDate{ComingSoon: coming, Date: v.Date})
	out := SteamReleaseFact{State: "unknown", ComingSoon: v.ComingSoon, RawText: v.Date, Precision: string(n.Precision), Year: n.Year, Month: n.Month, Quarter: n.Quarter}
	if v.ComingSoon != nil {
		if coming {
			out.State = "upcoming"
		} else {
			out.State = "available"
		}
	}
	if n.ExactDate != nil {
		out.ExactDate = n.ExactDate.Format("2006-01-02")
	}
	if n.RangeStart != nil {
		out.WindowStart = n.RangeStart.Format("2006-01-02")
		out.WindowEnd = n.RangeEnd.Format("2006-01-02")
	}
	return out
}
