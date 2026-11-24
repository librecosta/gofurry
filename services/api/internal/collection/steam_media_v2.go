package collection

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"errors"
	"net/url"
	"path"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/librecosta/gofurry/services/api/internal/catalog"
)

const (
	steamMediaRules = "gofurry-steam-media-2"
	steamMediaBytes = 32 << 10
)

var errMediaIdentity = errors.New("Harvest media AppID conflicts with registered Target")

func highFidelityMedia(o observation) bool {
	return o.Capability == "steam.assets" || o.Capability == intakeCapability && (o.Schema == 2 || o.Schema == 3) && o.Scope.Dimensions["section"] == "media"
}

func normalizeSteamHighFidelity(o observation, app string) (compact, error) {
	c := compact{Facts: map[string]jsontext.Value{}, Ruleset: steamMediaRules}
	if !steamScopeOK(o) || !scopePart.MatchString(o.ExecutionEgressRegion) || len(o.Data) > steamMediaBytes {
		return c, errProtocol
	}
	var value any
	var id uint32
	var valid bool
	switch o.Capability {
	case intakeCapability:
		var media catalog.SteamAppMediaV2
		if json.Unmarshal(o.Data, &media, json.RejectUnknownMembers(true)) != nil {
			return c, errProtocol
		}
		value, id = media, media.AppID
		valid = o.State == "present" && validDetailsMedia(media)
	case "steam.assets":
		var assets catalog.SteamAssetsV1
		if json.Unmarshal(o.Data, &assets, json.RejectUnknownMembers(true)) != nil {
			return c, errProtocol
		}
		value, id = assets, assets.AppID
		valid = validStoreAssets(o, assets)
	}
	if id != 0 && strconv.FormatUint(uint64(id), 10) != app {
		return c, errMediaIdentity
	}
	if id == 0 || !valid {
		return c, errProtocol
	}
	data, err := json.Marshal(value, json.Deterministic(true))
	if err != nil || len(data) > steamMediaBytes || !sameParams(data, o.Data) {
		return c, errProtocol
	}
	c.Data = data
	return c, nil
}

func validDetailsMedia(v catalog.SteamAppMediaV2) bool {
	if v.Images == nil || v.Truncated == nil {
		return false
	}
	img := *v.Images
	return (img != (catalog.SteamImagesV2{}) || len(v.Movies)+len(v.Screenshots) > 0) &&
		validMediaLinks("", img.Header, img.Capsule, img.CapsuleV5, img.Background, img.BackgroundRaw) &&
		validMediaItems(v.Movies, 12, func(m catalog.SteamMovie) (string, int, bool) {
			s := m.Sources
			return m.ID, m.Order, m.ID != "0" && len(m.Name) <= 256 &&
				!strings.ContainsFunc(m.Name, invalidMediaRune) && s != (catalog.SteamMovieSources{}) &&
				validMediaLinks("", m.Thumbnail) && validMediaLinks(".mp4", s.MP4480, s.MP4Max) &&
				validMediaLinks(".webm", s.WebM480, s.WebMMax) && validMediaLinks(".mpd", s.DASHAV1, s.DASHH264) &&
				validMediaLinks(".m3u8", s.HLSH264)
		}) && validMediaItems(v.Screenshots, 30, func(s catalog.SteamScreenshot) (string, int, bool) {
		return s.ID, s.Order, (s.Full != "" || s.Thumbnail != "") && validMediaLinks("", s.Full, s.Thumbnail)
	})
}

func validMediaItems[T any](items []T, limit int, fields func(T) (string, int, bool)) bool {
	if items == nil || len(items) > limit {
		return false
	}
	last := -1
	seen := make(map[string]bool, len(items))
	for _, item := range items {
		id, order, valid := fields(item)
		n, err := strconv.ParseUint(id, 10, 64)
		if !valid || err != nil || strconv.FormatUint(n, 10) != id || seen[id] || order <= last || order >= 512<<10 {
			return false
		}
		last, seen[id] = order, true
	}
	return true
}

func invalidMediaRune(r rune) bool { return unicode.IsControl(r) || unicode.Is(unicode.Cf, r) }

func parsedSteamCDN(raw string) *url.URL {
	if raw == "" || len(raw) > 2048 || strings.ContainsAny(raw, "#?\\") ||
		strings.ContainsFunc(raw, func(r rune) bool { return unicode.IsSpace(r) || invalidMediaRune(r) }) {
		return nil
	}
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.User != nil || u.Host != strings.ToLower(u.Hostname()) ||
		u.Host == "" || u.Path == "" || u.Path == "/" || u.String() != raw {
		return nil
	}
	return u
}

func validMediaLinks(format string, links ...string) bool {
	for _, link := range links {
		if link == "" {
			continue
		}
		u := parsedSteamCDN(link)
		if u == nil || !strings.HasSuffix(u.Host, ".steamstatic.com") && !strings.HasSuffix(u.Host, ".steamcontent.com") &&
			u.Host != "steamcdn-a.akamaihd.net" && u.Host != "steamuserimages-a.akamaihd.net" {
			return false
		}
		switch ext := strings.ToLower(path.Ext(u.Path)); ext {
		case ".mp4", ".webm", ".mpd", ".m3u8":
			if format != ext {
				return false
			}
		}
	}
	return true
}

// Order and aliases are part of Harvest's StoreBrowse wire contract.
var storeAssetTypes = [...]struct{ kind, family, slot, alias string }{
	{"library_capsule", "library", "library_capsule", ""},
	{"library_hero", "library", "library_hero", ""},
	{"library_logo", "library", "library_logo", "logo"},
	{"library_capsule_2x", "library", "library_capsule_2x", ""},
	{"library_hero_2x", "library", "library_hero_2x", ""},
	{"library_logo_2x", "library", "library_logo_2x", "logo_2x"},
	{"main_capsule", "store", "main_capsule", ""},
	{"small_capsule", "store", "small_capsule", ""},
	{"header", "store", "header", ""},
	{"main_capsule_2x", "store", "main_capsule_2x", ""},
	{"small_capsule_2x", "store", "small_capsule_2x", ""},
	{"header_2x", "store", "header_2x", ""},
	{"hero_capsule", "store", "hero_capsule", ""},
	{"hero_capsule_2x", "store", "hero_capsule_2x", ""},
	{"page_background", "store", "page_background", ""},
	{"page_background_raw", "store", "raw_page_background", ""},
	{"community_icon", "community", "community_icon", ""},
}

func validStoreAssets(o observation, set catalog.SteamAssetsV1) bool {
	if set.Assets == nil || set.Truncated == nil || len(set.Assets) > 32 {
		return false
	}
	if o.State == "confirmed_empty" {
		return len(set.Assets) == 0 && !*set.Truncated && o.Evidence == "store_assets_empty"
	}
	if o.State != "present" || len(set.Assets) == 0 {
		return false
	}
	app := strconv.FormatUint(uint64(set.AppID), 10)
	next := 0
	for _, a := range set.Assets {
		for next < len(storeAssetTypes) && storeAssetTypes[next].kind != a.Type {
			next++
		}
		if next == len(storeAssetTypes) {
			return false
		}
		rule := storeAssetTypes[next]
		next++
		sum := sha256.Sum256([]byte(strings.Join([]string{app, a.Slot, a.Locale, a.URL}, "\x00")))
		digest, ok := storeAssetURL(a.URL, app, rule.family == "community")
		if !ok || a.ID != hex.EncodeToString(sum[:]) || a.Digest != digest || a.Family != rule.family ||
			a.Slot == "" || a.Slot != rule.slot && a.Slot != rule.alias || a.Source != "store_browse" ||
			a.Locale != o.Scope.Locale || a.ObservedAt.Year() < 1970 || a.ObservedAt.Year() > 9999 ||
			!a.ObservedAt.Equal(set.Assets[0].ObservedAt) || a.ObservedAt.Sub(o.ObservedAt).Abs() > time.Microsecond {
			return false
		}
	}
	return true
}

func storeAssetURL(raw, app string, community bool) (string, bool) {
	u := parsedSteamCDN(raw)
	if u == nil {
		return "", false
	}
	tail := u.EscapedPath()
	if community {
		if u.Host != "cdn.cloudflare.steamstatic.com" {
			return "", false
		}
		tail = strings.TrimPrefix(tail, "/steamcommunity/public/images/apps/"+app+"/")
	} else {
		switch u.Host {
		case "shared.steamstatic.com", "shared.akamai.steamstatic.com", "shared.cloudflare.steamstatic.com",
			"shared.fastly.steamstatic.com", "shared.st.dl.eccdnx.com", "shared.cdn.steamchina.queniuam.com":
		default:
			return "", false
		}
		tail = strings.TrimPrefix(tail, "/store_item_assets/steam/apps/"+app+"/")
		tail = strings.TrimPrefix(tail, "/steam/apps/"+app+"/")
	}
	if tail == u.EscapedPath() || strings.Count(tail, "/") > 1 || community && strings.Contains(tail, "/") {
		return "", false
	}
	parts := strings.Split(tail, "/")
	for i, p := range parts {
		decoded, err := url.PathUnescape(p)
		if err != nil || !utf8.ValidString(decoded) || decoded == "" || decoded == "." || decoded == ".." ||
			strings.ContainsAny(decoded, "/\\%") || strings.ContainsFunc(decoded, invalidMediaRune) {
			return "", false
		}
		parts[i] = decoded
	}
	file := parts[len(parts)-1]
	ext := path.Ext(file)
	switch strings.ToLower(ext) {
	case ".jpg", ".jpeg", ".png", ".webp", ".avif", ".gif":
	default:
		return "", false
	}
	var digest string
	if community {
		if strings.ToLower(ext) != ".jpg" {
			return "", false
		}
		digest = strings.TrimSuffix(file, ext)
	} else if len(parts) == 2 {
		digest = parts[0]
	} else {
		return "", true
	}
	_, err := hex.DecodeString(digest)
	return strings.ToLower(digest), len(digest) == 40 && err == nil
}
