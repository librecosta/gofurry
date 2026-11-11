package collection

import (
	"context"
	json "encoding/json/v2"
	"errors"
	"net"
	"net/url"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
	"uuid"

	"github.com/jackc/pgx/v5"
	"github.com/librecosta/gofurry/services/api/internal/catalog"
	"github.com/librecosta/gofurry/services/api/internal/database/dbgen"
)

var ErrIntent = errors.New("collection requires a published record, active source and supported schedule")
var scopePart = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,34}$`)

func (s *Service) BindSite(ctx context.Context, publicID, address string) ([]dbgen.CollectionBinding, error) {
	if s.remote.writer == "" || !catalog.ValidPublicID(publicID) {
		return nil, ErrIntent
	}
	var bindings []dbgen.CollectionBinding
	err := s.locked(ctx, func(conn *pgx.Conn, _ *dbgen.Queries) error {
		return write(ctx, conn, func(q *dbgen.Queries) error {
			row, err := q.CollectionSiteAddress(ctx, dbgen.CollectionSiteAddressParams{PublicID: publicID, Url: address})
			if err != nil {
				return missingIntent(err)
			}
			u, err := url.Parse(row.Url)
			if err != nil || !strings.Contains(u.Hostname(), ".") || net.ParseIP(u.Hostname()) != nil {
				return ErrIntent
			}
			for _, source := range []struct{ resource, key, purpose string }{
				{"origin", u.Scheme + "://" + u.Host, row.Kind},
				{"domain", u.Hostname(), "domain"},
			} {
				identity, err := q.SaveCollectionSource(ctx, dbgen.SaveCollectionSourceParams{
					ID: uuid.NewV7(), Namespace: s.namespace, Provider: "web", ResourceType: source.resource, ExternalKey: source.key,
				})
				if err != nil {
					return err
				}
				binding, err := q.AddCollectionSiteBinding(ctx, dbgen.AddCollectionSiteBindingParams{
					ID: uuid.NewV7(), SourceID: identity.ID, ResourceType: source.resource, SiteID: &row.SiteID, AddressID: &row.ID, Purpose: source.purpose,
				})
				if err != nil {
					return err
				}
				bindings = append(bindings, binding)
			}
			return nil
		})
	})
	if err != nil {
		return nil, err
	}
	return bindings, nil
}

func (s *Service) BindGame(ctx context.Context, publicID, app string) (dbgen.CollectionBinding, error) {
	if s.remote.writer == "" || !catalog.ValidPublicID(publicID) || !steamID(app) {
		return dbgen.CollectionBinding{}, ErrIntent
	}
	var binding dbgen.CollectionBinding
	err := s.locked(ctx, func(conn *pgx.Conn, _ *dbgen.Queries) error {
		return write(ctx, conn, func(q *dbgen.Queries) error {
			row, err := q.CollectionSteamSource(ctx, dbgen.CollectionSteamSourceParams{PublicID: publicID, ExternalKey: app})
			if err != nil {
				return missingIntent(err)
			}
			source, err := q.SaveCollectionSource(ctx, dbgen.SaveCollectionSourceParams{
				ID: uuid.NewV7(), Namespace: s.namespace, Provider: "steam", ResourceType: "app", ExternalKey: row.ExternalKey,
			})
			if err != nil {
				return err
			}
			binding, err = q.AddCollectionGameBinding(ctx, dbgen.AddCollectionGameBindingParams{
				ID: uuid.NewV7(), SourceID: source.ID, GameID: &row.GameID, GameSourceID: &row.ID,
			})
			return err
		})
	})
	return binding, err
}

func steamID(s string) bool {
	v, err := strconv.ParseUint(s, 10, 32)
	return err == nil && v > 0 && strconv.FormatUint(v, 10) == s
}

func (s *Service) Unbind(ctx context.Context, id uuid.UUID) error {
	if s.remote.writer == "" || !v7(id) {
		return ErrIntent
	}
	return s.locked(ctx, func(conn *pgx.Conn, _ *dbgen.Queries) error {
		return write(ctx, conn, func(q *dbgen.Queries) error {
			if err := q.CloseCollectionBinding(ctx, dbgen.CloseCollectionBindingParams{ID: id, Namespace: s.namespace}); err != nil {
				return err
			}
			return q.CloseCollectionBindingCoverage(ctx, dbgen.CloseCollectionBindingCoverageParams{ID: id, Namespace: s.namespace})
		})
	})
}

type Plan struct {
	Capability   string
	Locale       string
	Region       string
	EgressRegion string
	RRTypes      []string
	NewsCount    int
	Interval     time.Duration
	Anchor       time.Time
}

type capabilitySpec struct {
	resource string
	minimum  time.Duration
	sections []string
}

var capabilities = map[string]capabilitySpec{
	"web.http":      {"origin", 15 * time.Minute, []string{"http", "html", "tls"}},
	"web.tls":       {"origin", 6 * time.Hour, []string{"tls"}},
	"web.dns":       {"domain", time.Hour, []string{"dns"}},
	"web.rdap":      {"domain", 7 * 24 * time.Hour, []string{"rdap"}},
	"steam.details": {"app", time.Hour, []string{"details", "price", "media"}},
	"steam.news":    {"app", time.Hour, []string{"news"}},
	"steam.players": {"app", 30 * time.Minute, []string{"players"}},
}

var dnsTypes = []string{"a", "aaaa", "caa", "cname", "mx", "ns", "soa", "txt"}

func (p Plan) params(resource, key string) ([]byte, []byte, string, error) {
	cap, ok := capabilities[p.Capability]
	fields := scope{strings.ReplaceAll(strings.ToLower(p.Locale), "_", "-"), strings.ToLower(p.Region), strings.ToLower(p.EgressRegion)}
	if !ok || resource != cap.resource || p.Interval < cap.minimum || p.Interval > 30*24*time.Hour || p.Interval%time.Second != 0 ||
		p.Anchor.Year() < 2000 || p.Anchor.Year() > 2100 || !scopePart.MatchString(fields.EgressRegion) ||
		(fields.Locale != "" && !scopePart.MatchString(fields.Locale)) || (fields.Region != "" && !scopePart.MatchString(fields.Region)) ||
		(p.Capability != "web.dns" && len(p.RRTypes) != 0) || (p.Capability != "steam.news" && p.NewsCount != 0) ||
		(p.Capability == "web.tls" && !strings.HasPrefix(key, "https://")) {
		return nil, nil, "", ErrIntent
	}
	var params any = struct{}{}
	policy := "state_refresh"
	switch p.Capability {
	case "web.dns":
		types := slices.Clone(p.RRTypes)
		if len(types) == 0 {
			types = slices.Clone(dnsTypes)
		}
		if len(types) > len(dnsTypes) {
			return nil, nil, "", ErrIntent
		}
		for i, rr := range types {
			types[i] = strings.ToLower(rr)
			if !slices.Contains(dnsTypes, types[i]) {
				return nil, nil, "", ErrIntent
			}
		}
		slices.Sort(types)
		params = struct {
			Types []string `json:"types"`
		}{slices.Compact(types)}
	case "steam.details":
		lang := "english"
		if fields.Locale == "zh-cn" {
			lang = "schinese"
		}
		if (fields.Locale != "en" && fields.Locale != "zh-cn") || !slices.Contains([]string{"cn", "us", "hk"}, fields.Region) {
			return nil, nil, "", ErrIntent
		}
		params = struct {
			Language string `json:"language"`
			Country  string `json:"store_country"`
		}{lang, strings.ToUpper(fields.Region)}
	case "steam.news", "steam.players":
		if fields.Locale != "" || fields.Region != "" {
			return nil, nil, "", ErrIntent
		}
		if p.Capability == "steam.players" {
			policy = "point_in_time"
			break
		}
		if p.NewsCount == 0 {
			p.NewsCount = 10
		}
		if p.NewsCount < 1 || p.NewsCount > 10 {
			return nil, nil, "", ErrIntent
		}
		params = struct {
			Count int `json:"count"`
		}{p.NewsCount}
	}
	paramJSON, _ := json.Marshal(params)
	scopeJSON, _ := json.Marshal(fields)
	return paramJSON, scopeJSON, policy, nil
}

func (s *Service) AddPlan(ctx context.Context, binding uuid.UUID, p Plan) (uuid.UUID, error) {
	if s.remote.writer == "" || !v7(binding) {
		return uuid.Nil(), ErrIntent
	}
	var id uuid.UUID
	err := s.locked(ctx, func(conn *pgx.Conn, _ *dbgen.Queries) error {
		return write(ctx, conn, func(q *dbgen.Queries) error {
			row, err := q.OpenCollectionBinding(ctx, dbgen.OpenCollectionBindingParams{ID: binding, Namespace: s.namespace})
			if err != nil {
				return missingIntent(err)
			}
			params, scope, policy, err := p.params(row.ResourceType, row.ExternalKey)
			if err != nil {
				return err
			}
			plan, err := q.SaveCollectionPlan(ctx, dbgen.SaveCollectionPlanParams{
				ID: uuid.NewV7(), SourceID: row.SourceID, Capability: p.Capability, Params: params, Scope: scope, Policy: policy,
				AnchorAt: timestamp(p.Anchor.Truncate(time.Microsecond)), IntervalSeconds: int32(p.Interval / time.Second),
			})
			if err != nil {
				return err
			}
			id = plan.ID
			owned, err := q.AttachCollectionPlan(ctx, dbgen.AttachCollectionPlanParams{PlanID: plan.ID, BindingID: binding, SourceID: row.SourceID})
			if err == nil && !owned {
				return ErrIntent
			}
			return err
		})
	})
	return id, err
}

func missingIntent(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrIntent
	}
	return err
}
