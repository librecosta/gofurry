package catalog

import (
	"context"
	"errors"
	"regexp"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"
	"uuid"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/librecosta/gofurry/services/api/internal/database/dbgen"
	"golang.org/x/text/language"
)

const NavigationVersion = 1

type NavigationTerm struct {
	Code string `json:"code"`
	ZH   string `json:"zh"`
	EN   string `json:"en"`
}

var categories = [...]NavigationTerm{
	{"community", "社区交流", "Communities"}, {"gallery", "作品展示", "Art & Galleries"},
	{"literature", "文学阅读", "Literature"}, {"resources", "工具资源", "Tools & Resources"},
	{"marketplace", "委托交易", "Commissions & Marketplace"}, {"knowledge", "资讯百科", "News & Knowledge"},
	{"gaming", "游戏互动", "Games & Interactive"}, {"events", "活动组织", "Events & Organizations"},
}
var navigationTags = [...]NavigationTerm{
	{"forum", "论坛", "Forum"}, {"portfolio", "作品集", "Portfolio"}, {"fursuit", "兽装", "Fursuit"},
	{"tutorial", "教程", "Tutorial"}, {"wiki", "维基", "Wiki"}, {"directory", "目录", "Directory"},
	{"podcast", "播客", "Podcast"}, {"webcomic", "网络漫画", "Webcomic"}, {"roleplay", "角色扮演", "Roleplay"},
	{"modding", "游戏模组", "Modding"}, {"convention", "展会", "Convention"}, {"translation", "翻译", "Translation"},
}

func NavigationCategories() []NavigationTerm { return slices.Clone(categories[:]) }
func NavigationTags() []NavigationTerm       { return slices.Clone(navigationTags[:]) }
func NavigationCategory(code string) bool    { return termExists(categories[:], code) }
func NavigationTag(code string) bool         { return termExists(navigationTags[:], code) }

func termExists(terms []NavigationTerm, code string) bool {
	return slices.ContainsFunc(terms, func(t NavigationTerm) bool { return t.Code == code })
}

var langCode = regexp.MustCompile(`^[a-z]{2,3}$`)
var actorCode = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.:@-]{0,79}$`)

func NavigationLanguage(code string) bool {
	if !langCode.MatchString(code) || code == "und" || code == "mul" || code == "zxx" || code >= "qaa" && code <= "qtz" {
		return false
	}
	tag, err := language.Parse(code)
	return err == nil && tag.String() == code
}

func navigationCodes(values []string, max int, valid func(string) bool) ([]string, bool) {
	if len(values) > max {
		return nil, false
	}
	out := append([]string{}, values...)
	slices.Sort(out)
	for i, v := range out {
		if !valid(v) || i > 0 && out[i-1] == v {
			return nil, false
		}
	}
	return out, true
}

func reviewText(actor, reason string) bool {
	return actorCode.MatchString(actor) && utf8.ValidString(reason) && utf8.RuneCountInString(reason) <= 400 &&
		strings.TrimSpace(reason) != "" && !strings.ContainsFunc(reason, unicode.IsControl)
}

type NavigationEdit struct {
	Rating, Primary, Actor, Reason string
	Secondary, Tags, Languages     []string
}

type NavigationProfile struct {
	PublicID  string   `json:"public_id"`
	Revision  int64    `json:"revision"`
	Rating    string   `json:"rating"`
	Primary   string   `json:"primary_category"`
	Secondary []string `json:"secondary_categories"`
	Tags      []string `json:"tags"`
	Languages []string `json:"languages"`
}

func profile(s dbgen.Site) NavigationProfile {
	return NavigationProfile{s.PublicID, s.Revision, s.ContentRating, s.PrimaryCategory.String, s.SecondaryCategories, s.NavigationTags, s.SupportedLanguages}
}

func (s *Service) InspectSiteNavigation(ctx context.Context, id string) (NavigationProfile, error) {
	if !ValidPublicID(id) {
		return NavigationProfile{}, ErrInvalid
	}
	return write(ctx, s, func(ctx context.Context, q *dbgen.Queries) (NavigationProfile, error) {
		row, err := q.FindYearbookSite(ctx, id)
		return profile(row), err
	})
}

func (s *Service) ReviewSiteNavigation(ctx context.Context, region, id string, revision int64, in NavigationEdit) (NavigationProfile, error) {
	invalid := NavigationProfile{}
	if !ValidPublicID(id) || revision < 1 || !reviewText(in.Actor, in.Reason) || region != "CN" && region != "INTL" {
		return invalid, ErrInvalid
	}
	switch in.Rating {
	case "sfw", "unreviewed":
	case "mixed", "adult":
		if region != "INTL" {
			return invalid, ErrInvalid
		}
	default:
		return invalid, ErrInvalid
	}
	secondary, ok := navigationCodes(in.Secondary, 2, NavigationCategory)
	if !ok || in.Primary == "" && len(secondary) > 0 ||
		in.Primary != "" && (!NavigationCategory(in.Primary) || slices.Contains(secondary, in.Primary)) {
		return invalid, ErrInvalid
	}
	tags, ok := navigationCodes(in.Tags, 8, NavigationTag)
	if !ok {
		return invalid, ErrInvalid
	}
	languages, ok := navigationCodes(in.Languages, 8, NavigationLanguage)
	if !ok {
		return invalid, ErrInvalid
	}
	return write(ctx, s, func(ctx context.Context, q *dbgen.Queries) (NavigationProfile, error) {
		before, err := q.LockNavigationSite(ctx, id)
		if err != nil {
			return invalid, err
		}
		if before.Revision != revision {
			return invalid, ErrConflict
		}
		if before.Status == Published && in.Rating == "sfw" && in.Primary == "" {
			return invalid, ErrInvalid
		}
		after, err := q.UpdateNavigationSite(ctx, dbgen.UpdateNavigationSiteParams{ID: before.ID, ContentRating: in.Rating,
			PrimaryCategory: pgtype.Text{String: in.Primary, Valid: in.Primary != ""}, SecondaryCategories: secondary,
			NavigationTags: tags, SupportedLanguages: languages})
		if err != nil {
			return invalid, err
		}
		err = q.AddNavigationReview(ctx, dbgen.AddNavigationReviewParams{ID: uuid.NewV7(), SiteID: before.ID,
			SiteRevision: after.Revision, PreviousRating: before.ContentRating, Rating: after.ContentRating, Actor: in.Actor, Reason: strings.TrimSpace(in.Reason)})
		return profile(after), err
	})
}

func SiteHistoryAllowed(site dbgen.Site) bool {
	return site.ContentRating == "sfw" || site.ContentRating == "unreviewed" && site.LegacyDetail
}

func (s *Service) NavigationFeatures(ctx context.Context) ([]dbgen.InspectNavigationFeaturesRow, error) {
	return write(ctx, s, func(ctx context.Context, q *dbgen.Queries) ([]dbgen.InspectNavigationFeaturesRow, error) {
		return q.InspectNavigationFeatures(ctx)
	})
}

func (s *Service) SetNavigationFeature(ctx context.Context, slot, publicID string, revision int64, actor, reason string) (int64, error) {
	if !slices.Contains([]string{"primary", "secondary_1", "secondary_2"}, slot) ||
		publicID != "" && !ValidPublicID(publicID) || revision < 0 || !reviewText(actor, reason) {
		return 0, ErrInvalid
	}
	return write(ctx, s, func(ctx context.Context, q *dbgen.Queries) (int64, error) {
		slots, err := q.LockNavigationFeatures(ctx)
		if err != nil {
			return 0, err
		}
		if len(slots) != 3 {
			return 0, errors.New("invalid fixed slot inventory")
		}
		var id *uuid.UUID
		if publicID != "" {
			site, err := q.LockNavigationSite(ctx, publicID)
			if err != nil {
				return 0, err
			}
			if site.Status != Published || site.ContentRating != "sfw" || !site.PrimaryCategory.Valid {
				return 0, ErrInvalid
			}
			id = &site.ID
		}
		for _, row := range slots {
			if row.Slot == slot && row.Revision != revision ||
				row.Slot != slot && id != nil && row.SiteID != nil && *row.SiteID == *id {
				return 0, ErrConflict
			}
		}
		result, err := q.UpdateNavigationFeature(ctx, dbgen.UpdateNavigationFeatureParams{Slot: slot, SiteID: id,
			Actor: pgtype.Text{String: actor, Valid: true}, Reason: pgtype.Text{String: strings.TrimSpace(reason), Valid: true}})
		return result.Revision, err
	})
}
