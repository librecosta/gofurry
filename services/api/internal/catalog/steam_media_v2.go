package catalog

import (
	"time"
	"uuid"
)

type SteamImagesV2 struct {
	Header        string `json:"header_url,omitempty"`
	Capsule       string `json:"capsule_url,omitempty"`
	CapsuleV5     string `json:"capsule_v5_url,omitempty"`
	Background    string `json:"background_url,omitempty"`
	BackgroundRaw string `json:"background_raw_url,omitempty"`
}

type SteamMovieSources struct {
	MP4480   string `json:"mp4_480,omitempty"`
	MP4Max   string `json:"mp4_max,omitempty"`
	WebM480  string `json:"webm_480,omitempty"`
	WebMMax  string `json:"webm_max,omitempty"`
	DASHAV1  string `json:"dash_av1,omitempty"`
	DASHH264 string `json:"dash_h264,omitempty"`
	HLSH264  string `json:"hls_h264,omitempty"`
}

type SteamMovie struct {
	ID        string            `json:"movie_id"`
	Name      string            `json:"name"`
	Thumbnail string            `json:"thumbnail_url,omitempty"`
	Highlight *bool             `json:"highlight,omitempty"`
	Order     int               `json:"order"`
	Sources   SteamMovieSources `json:"sources"`
}

type SteamScreenshot struct {
	ID        string `json:"screenshot_id"`
	Thumbnail string `json:"thumbnail_url,omitempty"`
	Full      string `json:"full_url,omitempty"`
	Order     int    `json:"order"`
}

type SteamAppMediaV2 struct {
	AppID       uint32            `json:"app_id"`
	Images      *SteamImagesV2    `json:"images"`
	Movies      []SteamMovie      `json:"movies"`
	Screenshots []SteamScreenshot `json:"screenshots"`
	Truncated   *bool             `json:"truncated"`
}

type SteamAsset struct {
	ID         string    `json:"asset_id"`
	Type       string    `json:"asset_type"`
	Family     string    `json:"asset_family"`
	Slot       string    `json:"source_slot"`
	URL        string    `json:"url"`
	Locale     string    `json:"locale"`
	Source     string    `json:"source_surface"`
	ObservedAt time.Time `json:"observed_at"`
	Digest     string    `json:"source_digest,omitempty"`
}

type SteamAssetsV1 struct {
	AppID     uint32       `json:"app_id"`
	Assets    []SteamAsset `json:"assets"`
	Truncated *bool        `json:"truncated"`
}

type GameMediaOrigin struct {
	GameSourceID          uuid.UUID `json:"game_source_id"`
	CollectionSourceID    uuid.UUID `json:"collection_source_id"`
	BindingID             uuid.UUID `json:"binding_id"`
	TargetID              uuid.UUID `json:"target_id"`
	Namespace             string    `json:"namespace"`
	Capability            string    `json:"capability"`
	OutputSchema          int32     `json:"output_schema"`
	Locale                string    `json:"locale"`
	StoreCountry          string    `json:"store_country"`
	EgressRegion          string    `json:"egress_region"`
	RequestedEgressRegion string    `json:"requested_egress_region"`
	ScopeKey              string    `json:"scope_key"`
	ObservationID         uuid.UUID `json:"observation_id"`
	ContentHash           string    `json:"content_hash"`
	Ruleset               string    `json:"ruleset_version"`
	ObservedAt            time.Time `json:"observed_at"`
	ConfirmedAt           time.Time `json:"confirmed_at"`
}

type GameMediaCandidateV2 struct {
	GameMediaOrigin
	State  string           `json:"state"`
	Media  *SteamAppMediaV2 `json:"media,omitempty"`
	Assets *SteamAssetsV1   `json:"assets,omitempty"`
}
