package catalog

import (
	"net/url"
	"path"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

const (
	publicMediaURLBytes    = 2048
	publicMediaImages      = 22
	publicMediaScreenshots = 8
	publicMediaMovies      = 4
)

// PublicGameMedia exposes the adopted media from one published locale.
type PublicGameMedia struct {
	Version     int                    `json:"version"`
	Locale      string                 `json:"locale"`
	Fallback    bool                   `json:"fallback"`
	Cover       *PublicGameMediaImage  `json:"cover,omitempty"`
	Images      []PublicGameMediaImage `json:"images"`
	Screenshots []PublicGameScreenshot `json:"screenshots"`
	Movies      []PublicGameMovie      `json:"movies"`
}

type PublicGameMediaImage struct {
	Type   string `json:"asset_type"`
	Family string `json:"asset_family"`
	URL    string `json:"url"`
}

type PublicGameScreenshot struct {
	ID        string `json:"screenshot_id"`
	Thumbnail string `json:"thumbnail_url,omitempty"`
	Full      string `json:"full_url,omitempty"`
}

type PublicGameMovie struct {
	ID        string            `json:"movie_id"`
	Name      string            `json:"name"`
	Thumbnail string            `json:"thumbnail_url,omitempty"`
	Highlight *bool             `json:"highlight,omitempty"`
	Sources   SteamMovieSources `json:"sources"`
}

func publishedProfileMedia(published []PublishedGameMedia, locale string) *PublicGameMedia {
	var fallback *PublishedGameMedia
	for i := range published {
		row := &published[i]
		if row.MediaV2 == nil || row.MediaV2.Version != 2 || row.Locale != "zh" && row.Locale != "en" {
			continue
		}
		if row.Locale == locale {
			return projectPublishedMedia(row.MediaV2, row.Locale, false)
		}
		fallback = row
	}
	if fallback == nil {
		return nil
	}
	return projectPublishedMedia(fallback.MediaV2, fallback.Locale, true)
}

func projectPublishedMedia(media *GameMediaSelectionV2, locale string, fallback bool) *PublicGameMedia {
	out := PublicGameMedia{
		Version: 2, Locale: locale, Fallback: fallback,
		Images:      make([]PublicGameMediaImage, 0, min(len(media.Images), publicMediaImages)),
		Screenshots: make([]PublicGameScreenshot, 0, min(len(media.Screenshots), publicMediaScreenshots)),
		Movies:      make([]PublicGameMovie, 0, min(len(media.Movies), publicMediaMovies)),
	}
	if media.Cover != nil {
		if cover, ok := publicMediaImage(*media.Cover); ok {
			out.Cover = &cover
		}
	}
	roles := make(map[string]bool, cap(out.Images))
	for _, image := range media.Images {
		item, ok := publicMediaImage(image)
		if !ok || roles[item.Type] {
			continue
		}
		roles[item.Type] = true
		out.Images = append(out.Images, item)
		if len(out.Images) == publicMediaImages {
			break
		}
	}
	for _, shot := range media.Screenshots {
		if !publicMediaID(shot.ID, true) {
			continue
		}
		thumb, full := publicMediaURL(shot.Thumbnail, ""), publicMediaURL(shot.Full, "")
		if thumb == "" && full == "" {
			continue
		}
		out.Screenshots = append(out.Screenshots, PublicGameScreenshot{ID: shot.ID, Thumbnail: thumb, Full: full})
		if len(out.Screenshots) == publicMediaScreenshots {
			break
		}
	}
	for _, movie := range media.Movies {
		if !publicMediaID(movie.ID, false) {
			continue
		}
		name := movie.Name
		if len(name) > 256 || !utf8.ValidString(name) || strings.ContainsFunc(name, unsafeMediaRune) {
			name = ""
		}
		s := movie.Sources
		out.Movies = append(out.Movies, PublicGameMovie{
			ID: movie.ID, Name: name, Thumbnail: publicMediaURL(movie.Thumbnail, ""), Highlight: movie.Highlight,
			Sources: SteamMovieSources{
				MP4480: publicMediaURL(s.MP4480, ".mp4"), MP4Max: publicMediaURL(s.MP4Max, ".mp4"),
				WebM480: publicMediaURL(s.WebM480, ".webm"), WebMMax: publicMediaURL(s.WebMMax, ".webm"),
				HLSH264:  publicMediaURL(s.HLSH264, ".m3u8"),
				DASHH264: publicMediaURL(s.DASHH264, ".mpd"), DASHAV1: publicMediaURL(s.DASHAV1, ".mpd"),
			},
		})
		if len(out.Movies) == publicMediaMovies {
			break
		}
	}
	if out.Cover == nil && len(out.Images)+len(out.Screenshots)+len(out.Movies) == 0 {
		return nil
	}
	return &out
}

func publicMediaImage(image GameSelectedImage) (PublicGameMediaImage, bool) {
	var family string
	switch image.Type {
	case "library_capsule", "library_hero", "library_logo", "library_capsule_2x", "library_hero_2x", "library_logo_2x":
		family = "library"
	case "community_icon":
		family = "community"
	case "header", "header_2x", "capsule", "capsule_v5", "main_capsule", "main_capsule_2x", "small_capsule", "small_capsule_2x",
		"hero_capsule", "hero_capsule_2x", "background", "background_raw", "page_background", "page_background_raw":
		family = "store"
	default:
		return PublicGameMediaImage{}, false
	}
	link := publicMediaURL(image.URL, "")
	return PublicGameMediaImage{Type: image.Type, Family: family, URL: link}, image.Family == family && link != ""
}

func publicMediaID(value string, allowZero bool) bool {
	n, err := strconv.ParseUint(value, 10, 64)
	return err == nil && (allowZero || n > 0) && strconv.FormatUint(n, 10) == value
}

func unsafeMediaRune(r rune) bool { return unicode.IsControl(r) || unicode.Is(unicode.Cf, r) }

// An empty format denotes a static image. StoreBrowse's additional mirrors
// are limited to their known static app-asset paths.
func publicMediaURL(raw, format string) string {
	if raw == "" || len(raw) > publicMediaURLBytes || strings.ContainsAny(raw, "#?\\<>") ||
		strings.ContainsFunc(raw, func(r rune) bool { return unsafeMediaRune(r) || unicode.IsSpace(r) }) {
		return ""
	}
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.User != nil || u.Opaque != "" || !strings.HasPrefix(u.Path, "/") {
		return ""
	}
	host := strings.ToLower(u.Hostname())
	if !strings.EqualFold(u.Host, host) && !strings.EqualFold(u.Host, host+":443") {
		return ""
	}
	if strings.HasPrefix(host, ".") || strings.Contains(host, "..") {
		return ""
	}
	switch host {
	case "shared.st.dl.eccdnx.com", "shared.cdn.steamchina.queniuam.com":
		assetPath := strings.HasPrefix(u.EscapedPath(), "/store_item_assets/steam/apps/") ||
			strings.HasPrefix(u.EscapedPath(), "/steam/apps/")
		if format != "" || !assetPath {
			return ""
		}
	case "steamcdn-a.akamaihd.net", "steamuserimages-a.akamaihd.net":
	default:
		if !strings.HasSuffix(host, ".steamstatic.com") && !strings.HasSuffix(host, ".steamcontent.com") {
			return ""
		}
	}
	for segment := range strings.SplitSeq(u.EscapedPath()[1:], "/") {
		decoded, err := url.PathUnescape(segment)
		if err != nil || !utf8.ValidString(decoded) || decoded == "" || decoded == "." || decoded == ".." ||
			strings.ContainsAny(decoded, "/\\%<>") || strings.ContainsFunc(decoded, unsafeMediaRune) {
			return ""
		}
	}
	ext := strings.ToLower(path.Ext(u.Path))
	if format != "" {
		if ext != format {
			return ""
		}
	} else {
		switch ext {
		case ".jpg", ".jpeg", ".png", ".webp", ".avif", ".gif":
		default:
			return ""
		}
	}
	link := u.String()
	if len(link) > publicMediaURLBytes {
		return ""
	}
	return link
}
