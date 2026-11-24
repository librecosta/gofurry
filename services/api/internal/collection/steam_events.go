package collection

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"errors"
	"strconv"
	"strings"
	"time"
	"unicode"
	"uuid"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/librecosta/gofurry/services/api/internal/catalog"
	"github.com/librecosta/gofurry/services/api/internal/database/dbgen"
)

const steamEventsCapability = "steam.events"
const steamEventsLimit = 480 << 10

type steamEvent struct {
	EventGID        string   `json:"event_gid"`
	AnnouncementGID string   `json:"announcement_gid"`
	ForumTopicID    string   `json:"forum_topic_id,omitempty"`
	ClanID          string   `json:"clan_steam_id,omitempty"`
	Type            *int     `json:"event_type,omitzero"`
	Name            string   `json:"event_name"`
	Headline        string   `json:"headline"`
	Body            string   `json:"raw_body"`
	OriginalURL     string   `json:"original_url,omitempty"`
	URL             string   `json:"url"`
	Language        *int     `json:"language,omitzero"`
	LanguageState   string   `json:"language_state"`
	Published       *bool    `json:"published,omitzero"`
	Hidden          bool     `json:"hidden"`
	Tags            []string `json:"tags"`
	Comments        *int     `json:"comment_count,omitzero"`
	VotesUp         *int     `json:"vote_up_count,omitzero"`
	VotesDown       *int     `json:"vote_down_count,omitzero"`
	Post            *int64   `json:"post_time,omitzero"`
	Update          *int64   `json:"update_time,omitzero"`
	Start           *int64   `json:"start_time,omitzero"`
	End             *int64   `json:"end_time,omitzero"`
	Created         *int64   `json:"created_time,omitzero"`
	Modified        *int64   `json:"modified_time,omitzero"`
}

type steamEventsDiagnostic struct {
	EventGID string `json:"event_gid,omitempty"`
	Code     string `json:"code"`
}

type steamEventPage struct {
	AppID           uint32                  `json:"app_id"`
	RequestedLocale string                  `json:"requested_locale"`
	Events          []steamEvent            `json:"events"`
	Diagnostics     []steamEventsDiagnostic `json:"diagnostics,omitempty"`
	Truncated       bool                    `json:"truncated"`
}

func steamEvents(o observation) bool {
	return o.Capability == steamEventsCapability && o.Schema == 1 && o.Scope.Dimensions["section"] == "events"
}

func decimalSteamID(s string) bool {
	i, err := strconv.ParseUint(s, 10, 64)
	return err == nil && i > 0 && strconv.FormatUint(i, 10) == s
}

func eventString(s string, max int) bool {
	return len(s) <= max && !strings.ContainsRune(s, 0)
}

func eventLanguage(e steamEvent, request string) string {
	if e.Language == nil {
		return "unknown"
	}
	if *e.Language == 0 && request == "en" || *e.Language == 6 && request == "zh-cn" {
		return "declared_match"
	}
	if *e.Language == 0 && request == "zh-cn" {
		return "fallback"
	}
	return "different"
}

func normalizeSteamEvents(o observation, app string) (compact, error) {
	c := compact{Facts: map[string]jsontext.Value{}, Ruleset: "gofurry-steam-events-1"}
	var p steamEventPage
	if !steamEvents(o) || !scopePart.MatchString(o.ExecutionEgressRegion) || o.Scope.Region != "" ||
		(o.Scope.Locale != "en" && o.Scope.Locale != "zh-cn") || len(o.Data) > steamEventsLimit ||
		json.Unmarshal(o.Data, &p, json.RejectUnknownMembers(true)) != nil || p.Events == nil ||
		p.RequestedLocale != o.Scope.Locale || len(p.Events) > 5 || len(p.Diagnostics) > 6 ||
		(o.State == "confirmed_empty") != (o.Evidence == "empty_events" && len(p.Events) == 0 && len(p.Diagnostics) == 0) {
		return c, errProtocol
	}
	if strconv.FormatUint(uint64(p.AppID), 10) != app {
		return c, errMediaIdentity
	}
	seen := make(map[string]bool)
	for _, d := range p.Diagnostics {
		if !evidenceCode.MatchString(d.Code) || d.EventGID != "" && !decimalSteamID(d.EventGID) {
			return c, errProtocol
		}
	}
	for _, e := range p.Events {
		if !decimalSteamID(e.EventGID) || !decimalSteamID(e.AnnouncementGID) || seen[e.EventGID] ||
			e.URL != "https://store.steampowered.com/news/app/"+app+"/view/"+e.AnnouncementGID {
			return c, errMediaIdentity
		}
		seen[e.EventGID] = true
		if !validSteamEvent(e, p.RequestedLocale) {
			return c, errProtocol
		}
	}
	c.Data = o.Data
	return c, nil
}

func validSteamEvent(e steamEvent, locale string) bool {
	if strings.TrimSpace(e.Headline) == "" || strings.TrimSpace(e.Body) == "" || !eventString(e.Headline, 2048) ||
		!eventString(e.Body, 128<<10) || !eventString(e.Name, 2048) || !eventString(e.OriginalURL, 2048) ||
		e.LanguageState != eventLanguage(e, locale) || len(e.Tags) > 64 {
		return false
	}
	for _, tag := range e.Tags {
		if !eventString(tag, 128) {
			return false
		}
	}
	for _, id := range []string{e.ForumTopicID, e.ClanID} {
		if id != "" && !decimalSteamID(id) {
			return false
		}
	}
	for _, n := range []*int{e.Type, e.Language, e.Comments, e.VotesUp, e.VotesDown} {
		if n != nil && (*n < 0 || *n > 2147483647) {
			return false
		}
	}
	for _, n := range []*int64{e.Post, e.Update, e.Start, e.End, e.Created, e.Modified} {
		if n != nil && (*n < 0 || *n > 4102444800) {
			return false
		}
	}
	return true
}

func steamEventTime(n *int64) pgtype.Timestamptz {
	if n == nil || *n == 0 {
		return pgtype.Timestamptz{}
	}
	return timestamp(time.Unix(*n, 0))
}

func (s *Service) saveSteamEvents(ctx context.Context, q *dbgen.Queries, generation uuid.UUID, source *uuid.UUID, scope string) error {
	filter := dbgen.EligibleSteamEventObservationsParams{
		Generation: generation, Namespace: s.namespace, SourceID: source, FilterScope: scope,
	}
	for {
		rows, err := q.EligibleSteamEventObservations(ctx, filter)
		if err != nil {
			return err
		}
		if len(rows) == 0 {
			return nil
		}
		for _, r := range rows {
			var batch steamEventPage
			if err := json.Unmarshal(r.Data, &batch); err != nil || r.TargetID == nil || r.ObservationID == nil {
				return errProtocol
			}
			for _, e := range batch.Events {
				if err := saveSteamEvent(ctx, q, r, batch, e); err != nil {
					return err
				}
			}
		}
		last := rows[len(rows)-1]
		filter.AfterGameSource, filter.AfterScope, filter.AfterEgress = last.GameSourceID, last.ScopeKey, last.EgressRegion
	}
}

func saveSteamEvent(ctx context.Context, q *dbgen.Queries, r dbgen.EligibleSteamEventObservationsRow, batch steamEventPage, event steamEvent) error {
	version, err := json.Marshal([]string{event.Headline, event.Body})
	if err != nil {
		return err
	}
	digest := sha256.Sum256(version)
	hash := hex.EncodeToString(digest[:])
	language := int32(-1)
	if event.Language != nil {
		language = int32(*event.Language)
	}
	body := event.Body
	html, plain, summary := catalog.CleanSteamEventBody(body)
	bodyLanguageSignal := "unverified"
	if strings.ContainsFunc(plain, func(r rune) bool { return unicode.Is(unicode.Han, r) }) {
		bodyLanguageSignal = "han_present"
	}
	event.Body = ""
	metrics, err := json.Marshal(struct {
		Comments  *int `json:"comment_count,omitzero"`
		VotesUp   *int `json:"vote_up_count,omitzero"`
		VotesDown *int `json:"vote_down_count,omitzero"`
	}{event.Comments, event.VotesUp, event.VotesDown})
	if err != nil {
		return err
	}
	event.Comments, event.VotesUp, event.VotesDown = nil, nil, nil
	metadata, err := json.Marshal(struct {
		steamEvent
		Truncated          bool                    `json:"truncated"`
		Diagnostics        []steamEventsDiagnostic `json:"diagnostics,omitempty"`
		BodyLanguageSignal string                  `json:"body_language_signal"`
	}{event, batch.Truncated, batch.Diagnostics, bodyLanguageSignal})
	if err != nil {
		return err
	}
	post := steamEventTime(event.Post)
	update := event.Update
	if event.Modified != nil && (update == nil || *event.Modified > *update) {
		update = event.Modified
	}
	updated := steamEventTime(update)
	id, err := q.UpsertSteamEvent(ctx, dbgen.UpsertSteamEventParams{
		ID: uuid.NewV7(), GameSourceID: r.GameSourceID, CollectionSourceID: r.CollectionSourceID, BindingID: r.BindingID,
		TargetID: *r.TargetID, Namespace: r.Namespace, ScopeKey: r.ScopeKey, RequestedLocale: r.RequestedLocale, EgressRegion: r.EgressRegion,
		ObservationID: *r.ObservationID, ContentHash: r.ContentHash, ObservedAt: r.ObservedAt, ConfirmedAt: r.ConfirmedAt,
		EventGid: event.EventGID, AnnouncementGid: event.AnnouncementGID, Language: language, Headline: event.Headline,
		PublishedAt: post, UpdatedAt: updated, Eligible: (language == 0 || language == 6) && event.Published != nil && *event.Published &&
			!event.Hidden && post.Valid && html != "",
		BodyHash: hash, Metadata: metadata, Metrics: metrics,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	if err := q.WriteSteamEventVersion(ctx, dbgen.WriteSteamEventVersionParams{
		EventID: id, BodyHash: hash, Headline: event.Headline, RawBody: body, Html: html, PlainText: plain, Summary: summary,
		ObservationID: *r.ObservationID, ObservedAt: r.ObservedAt, UpdatedAt: updated,
	}); err != nil {
		return err
	}
	return q.BoundSteamEventVersions(ctx, dbgen.BoundSteamEventVersionsParams{EventID: id, BodyHash: hash})
}
