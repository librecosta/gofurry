package catalog

import (
	"context"
	"net/url"
	"path"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/librecosta/gofurry/services/api/internal/database/dbgen"
)

type GameProfile struct {
	Summary
	PublishedDetails *PublicGameRichDetails `json:"published_details,omitempty"`
	PublishedMedia   *PublicGameMedia       `json:"published_media,omitempty"`
	SteamFacts       *PublicSteamFacts      `json:"steam_facts,omitempty"`
	CoverURL         string                 `json:"cover_url,omitempty"`
	Sources          []GameProfileSource    `json:"sources"`
	News             ChannelPage[GameStory] `json:"news"`
	NewsStatus       string                 `json:"news_status"`
	Related          []RelatedGame          `json:"related"`
}

type RelatedGame struct {
	Summary
	CoverURL string `json:"cover_url,omitempty"`
}

type GameProfileSource struct {
	Source
	Active    bool          `json:"active"`
	Preferred bool          `json:"preferred"`
	Details   *StoreDetails `json:"details,omitempty"`
	Media     *StoreMedia   `json:"media,omitempty"`
	Quotes    []StoreQuote  `json:"quotes"`
	Players   *PlayerSample `json:"players,omitempty"`
}

type StoreEvidence struct {
	Locale       string    `json:"locale"`
	StoreCountry string    `json:"store_country"`
	ObservedAt   time.Time `json:"observed_at"`
	Status       string    `json:"status"`
}

type StoreDetails struct {
	StoreEvidence
	SteamDetails
	GenreCodes       []string `json:"genre_codes"`
	ReleasePrecision string   `json:"release_precision"`
	ReleaseDay       string   `json:"release_day,omitempty"`
}

type StoreMedia struct {
	StoreEvidence
	SteamMedia
}

type StoreQuote struct {
	Locale       string        `json:"locale"`
	StoreCountry string        `json:"store_country"`
	Currency     string        `json:"currency"`
	State        string        `json:"state"`
	Amounts      *SteamAmounts `json:"amounts"`
	ObservedAt   time.Time     `json:"observed_at"`
	ConfirmedAt  time.Time     `json:"confirmed_at"`
}

type PlayerSample struct {
	Count      int64     `json:"count"`
	ObservedAt time.Time `json:"observed_at"`
}

func steamLocale(locale string) string {
	if locale == "zh" {
		return "zh-cn"
	}
	return "en"
}

func storeCurrency(country string) string {
	switch country {
	case "cn":
		return "CNY"
	case "us":
		return "USD"
	case "hk":
		return "HKD"
	}
	return ""
}

func validAppID(id string) bool {
	n, err := strconv.ParseUint(id, 10, 32)
	return err == nil && n > 0 && strconv.FormatUint(n, 10) == id
}

func officialGameURL(source Source) string {
	switch source.PlatformCode {
	case "steam":
		if validAppID(source.ExternalKey) {
			return "https://store.steampowered.com/app/" + source.ExternalKey + "/"
		}
	case "itch_io":
		address := PublicVisitURL(source.SourceURL)
		u, err := url.Parse(address)
		if err == nil && u.Scheme == "https" && u.Host == u.Hostname() && u.RawQuery == "" && !u.ForceQuery &&
			strings.HasSuffix(u.Host, ".itch.io") && strings.Count(u.Host, ".") == 2 &&
			u.Path != "/" && strings.Count(strings.TrimSuffix(u.Path, "/"), "/") == 1 {
			return address
		}
	}
	return ""
}

func profileMedia(raw string, video bool) string {
	raw = SteamMediaURL(raw)
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	ext := strings.ToLower(path.Ext(u.Path))
	if video && (ext == ".mp4" || ext == ".webm") ||
		!video && slices.Contains([]string{".jpg", ".jpeg", ".png", ".webp", ".avif"}, ext) {
		return raw
	}
	return ""
}

func profileMediaList(items []string, video bool, limit int) []string {
	out := []string{}
	for _, raw := range items {
		if safe := profileMedia(raw, video); safe != "" && !slices.Contains(out, safe) {
			out = append(out, safe)
			if len(out) == limit {
				break
			}
		}
	}
	return out
}

func preferredSteamDetails(rows []SteamMetadata, locale, country string) *SteamMetadata {
	var chosen *SteamMetadata
	score := 0
	for i := range rows {
		item := &rows[i]
		if item.Section != "details" || item.Details == nil {
			continue
		}
		rank := 1
		if item.StoreCountry == country {
			rank += 2
		}
		if item.Locale == locale && (locale != "zh-cn" || item.VerifiedChinese) {
			rank += 4
		} else if item.Locale == "en" {
			rank++
		}
		if chosen == nil || rank > score || rank == score && item.ObservedAt.After(chosen.ObservedAt) {
			chosen, score = item, rank
		}
	}
	return chosen
}

func readStoreEvidence(m *SteamMetadata) StoreEvidence {
	locale := m.Locale
	if m.Section == "details" && locale == "zh-cn" && !m.VerifiedChinese {
		locale = "und"
	}
	return StoreEvidence{Locale: locale, StoreCountry: m.StoreCountry, ObservedAt: m.ObservedAt, Status: m.State}
}

func projectGameSource(item GameSourceSnapshot, preferred bool, locale, country string) GameProfileSource {
	out := GameProfileSource{Source: item.Source, Active: item.Active, Preferred: preferred, Quotes: []StoreQuote{}}
	out.SourceURL = officialGameURL(item.Source)
	if item.PlatformCode != "steam" || !validAppID(item.ExternalKey) {
		return out
	}
	if selected := preferredSteamDetails(item.Metadata, locale, country); selected != nil {
		details := *selected.Details
		details.Content = nil
		details.Name, details.Description = "", ""
		out.Details = &StoreDetails{StoreEvidence: readStoreEvidence(selected), SteamDetails: details, GenreCodes: selected.GenreCodes, ReleasePrecision: "unknown"}
		if selected.ReleasePrecision != nil {
			out.Details.ReleasePrecision = *selected.ReleasePrecision
		}
		if selected.ReleaseDay != nil {
			out.Details.ReleaseDay = *selected.ReleaseDay
		}
	}
	for _, market := range []string{country, "cn", "us", "hk"} {
		if slices.ContainsFunc(out.Quotes, func(q StoreQuote) bool { return q.StoreCountry == market }) {
			continue
		}
		currency := storeCurrency(market)
		var picked *SteamPrice
		for i := range item.Prices {
			quote := &item.Prices[i]
			if quote.StoreCountry != market || quote.ObservedAt.After(quote.ConfirmedAt) ||
				!validStoreQuote(quote, currency) {
				continue
			}
			if picked == nil || (quote.Locale == locale && picked.Locale != locale) ||
				(quote.Locale == locale) == (picked.Locale == locale) && quote.ConfirmedAt.After(picked.ConfirmedAt) {
				picked = quote
			}
		}
		if picked != nil {
			out.Quotes = append(out.Quotes, StoreQuote{Locale: picked.Locale, StoreCountry: market, Currency: currency,
				State: picked.State, Amounts: picked.Amounts, ObservedAt: picked.ObservedAt, ConfirmedAt: picked.ConfirmedAt})
		}
	}
	if item.Players != nil {
		out.Players = &PlayerSample{Count: item.Players.Count, ObservedAt: item.Players.ObservedAt}
	}
	return out
}

func validStoreQuote(q *SteamPrice, currency string) bool {
	if currency == "" {
		return false
	}
	if q.State == "free" {
		return q.Amounts == nil
	}
	a := q.Amounts
	return a != nil && a.Currency == currency && a.Initial != nil && a.Final != nil && *a.Initial >= *a.Final &&
		a.Discount != nil && *a.Discount >= 0 && *a.Discount <= 100 &&
		(q.State == "priced" && *a.Final > 0 || q.State == "zero_price" && *a.Final == 0)
}

func (s *Service) PublicGameProfile(ctx context.Context, market ChannelMarket, id, locale string) (GameProfile, error) {
	if !ValidPublicID(id) {
		return GameProfile{}, ErrNotFound
	}
	country, _, egress := market.scope()
	if (locale != "en" && locale != "zh") || !steamEgress.MatchString(egress) {
		return GameProfile{}, ErrInvalid
	}
	if s.pool == nil {
		return GameProfile{}, ErrUnavailable
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	tx, q, err := s.channelRead(ctx)
	if err != nil {
		return GameProfile{}, err
	}
	defer tx.Rollback(ctx)
	p, err := readGamePresentation(ctx, q, id, egress, false)
	if err != nil {
		return GameProfile{}, err
	}
	out := GameProfile{
		Summary: p.Summary, PublishedMedia: publishedProfileMedia(p.Presentation, locale),
		PublishedDetails: publishedGameRichDetails(p.Presentation, locale),
		Sources:          []GameProfileSource{}, Related: []RelatedGame{}, NewsStatus: "available",
	}
	facts, err := readSteamFacts(ctx, q, id, egress, country, steamLocale(locale), false)
	if err != nil {
		return GameProfile{}, classify(err)
	}
	if facts.AppID != "" && (len(facts.Sources)+len(facts.Prices) > 0 || facts.Players.Samples > 0 || facts.FirstAvailable != nil) {
		recent, err := steamChanges(ctx, q, id, egress, SteamChangeFilter{Market: country}, true)
		if err != nil {
			return GameProfile{}, classify(err)
		}
		for _, row := range recent.Items[:min(5, len(recent.Items))] {
			facts.Changes = append(facts.Changes, publicSteamChange(row))
		}
		out.SteamFacts = &facts.PublicSteamFacts
	}
	// Keep the publication-selected source first, irrespective of quote freshness.
	if p.PreferredSource != nil {
		slices.SortStableFunc(p.Sources, func(a, b GameSourceSnapshot) int {
			switch {
			case a.ID == b.ID:
				return 0
			case a.ID == *p.PreferredSource:
				return -1
			case b.ID == *p.PreferredSource:
				return 1
			default:
				return 0
			}
		})
	}
	var chosen *PublishedGameMedia
	for i := range p.Presentation {
		media := &p.Presentation[i]
		if media.MediaV2 != nil {
			origin := media.MediaV2.displayOrigin()
			media.SourceID, media.ObservedAt, media.StoreCountry = &origin.GameSourceID, &origin.ObservedAt, origin.StoreCountry
			media.Media = media.MediaV2.legacyMedia(origin.GameSourceID)
		}
		if media.SourceID != nil && (chosen == nil || media.Locale == locale) {
			chosen = media
		}
		if media.Locale == locale {
			out.CoverURL = media.displayCover()
		}
	}
	if out.CoverURL == "" {
		for _, media := range p.Presentation {
			if link := media.displayCover(); link != "" {
				out.CoverURL = link
				break
			}
		}
	}
	for _, source := range p.Sources {
		item := projectGameSource(source, p.PreferredSource != nil && source.ID == *p.PreferredSource, steamLocale(locale), country)
		if chosen != nil && chosen.SourceID != nil && *chosen.SourceID == source.ID && chosen.ObservedAt != nil &&
			(chosen.displayCover() != "" || chosen.Media.Header != "" || chosen.Media.Capsule != "" || len(chosen.Media.Screenshots)+len(chosen.Media.Movies) > 0) {
			media := chosen.Media
			if chosen.MediaV2 == nil {
				media = reviewedSteamMedia(media)
			}
			item.Media = &StoreMedia{StoreEvidence: StoreEvidence{Locale: steamLocale(chosen.Locale), StoreCountry: chosen.StoreCountry, ObservedAt: *chosen.ObservedAt, Status: "observed"}, SteamMedia: media}
		}
		out.Sources = append(out.Sources, item)
	}
	rows, err := q.RelatedGameProfiles(ctx, dbgen.RelatedGameProfilesParams{PublicID: id, Locale: steamLocale(locale)})
	if err != nil {
		return GameProfile{}, classify(err)
	}
	for _, row := range rows {
		item, err := summary(row.PublicID, row.Slug, row.Localizations)
		if err != nil {
			return GameProfile{}, err
		}
		out.Related = append(out.Related, RelatedGame{Summary: item, CoverURL: profileMedia(row.Cover, false)})
	}
	out.News, err = gameProfileNews(ctx, q, id, "", egress, locale, "", 6)
	if err != nil {
		return GameProfile{}, err
	}
	return out, classify(tx.Commit(ctx))
}
