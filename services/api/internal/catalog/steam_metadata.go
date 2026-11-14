package catalog

// SteamDetails describes what the scoped Storefront observation reported.
// A nil boolean means Steam omitted that field.
type SteamDetails struct {
	Name               string          `json:"name"`
	Type               string          `json:"type,omitempty"`
	Description        string          `json:"description,omitempty"`
	SupportedLanguages string          `json:"supported_languages,omitempty"`
	Developers         []string        `json:"developers,omitempty"`
	Publishers         []string        `json:"publishers,omitempty"`
	Platforms          *SteamPlatforms `json:"platforms,omitempty"`
	Categories         []SteamLabel    `json:"categories,omitempty"`
	Genres             []SteamLabel    `json:"genres,omitempty"`
	Release            *SteamRelease   `json:"release,omitempty"`
	Content            *SteamContent   `json:"content_rating,omitempty"`
}

type SteamContent struct {
	RequiredAge *int   `json:"required_age,omitempty"`
	ESRB        string `json:"esrb,omitempty"`
	PEGI        string `json:"pegi,omitempty"`
	Descriptors []int  `json:"descriptors,omitempty"`
}

type SteamPlatforms struct {
	Windows *bool `json:"windows,omitempty"`
	Mac     *bool `json:"mac,omitempty"`
	Linux   *bool `json:"linux,omitempty"`
}

type SteamLabel struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type SteamRelease struct {
	ComingSoon *bool  `json:"coming_soon,omitempty"`
	DateText   string `json:"date_text,omitempty"`
}

type SteamMedia struct {
	Header      string   `json:"header,omitempty"`
	Capsule     string   `json:"capsule,omitempty"`
	Screenshots []string `json:"screenshots,omitempty"`
	Movies      []string `json:"movies,omitempty"`
}

type SteamAmounts struct {
	Currency string `json:"currency"`
	Initial  *int64 `json:"initial"`
	Final    *int64 `json:"final"`
	Discount *int   `json:"discount_percent"`
}
