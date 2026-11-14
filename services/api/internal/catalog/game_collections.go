package catalog

import (
	"context"
	json "encoding/json/v2"
	"errors"
	"strings"
	"time"
	"uuid"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/librecosta/gofurry/services/api/internal/database/dbgen"
)

type CollectionSummary struct {
	Summary
	FirstPublishedAt *time.Time `json:"first_published_at"`
}
type PublicCollection struct {
	CollectionSummary
	Members ChannelPage[GameCard] `json:"members"`
}
type CollectionRecord struct {
	CollectionSummary
	Status   string   `json:"status"`
	Revision int64    `json:"revision"`
	Members  []string `json:"members"`
}

func collectionSummary(id, slug string, text []byte, first pgtype.Timestamptz) (CollectionSummary, error) {
	info, err := summary(id, slug, text)
	result := CollectionSummary{Summary: info}
	if first.Valid {
		result.FirstPublishedAt = &first.Time
	}
	return result, err
}

func (s *Service) GameCollections(ctx context.Context, market ChannelMarket, c ChannelCriteria) (ChannelPage[CollectionSummary], error) {
	if err := c.normalize("collections"); err != nil {
		return ChannelPage[CollectionSummary]{}, err
	}
	pos, err := newChannelCursor(c, market, "collections", "", 0)
	if err != nil {
		return ChannelPage[CollectionSummary]{}, err
	}
	if s.pool == nil {
		return ChannelPage[CollectionSummary]{}, ErrUnavailable
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	rows, err := dbgen.New(s.pool).ListGameCollections(ctx, dbgen.ListGameCollectionsParams{
		AsOf: channelTime(pos.AsOf), HasAfter: pos.ID != "", AfterID: pos.ID, AfterTime: channelTime(pos.Ordering), Keyword: c.Keyword, TakeRows: c.Limit + 1,
	})
	if err != nil {
		return ChannelPage[CollectionSummary]{}, classify(err)
	}
	out := ChannelPage[CollectionSummary]{Items: make([]CollectionSummary, 0, min(int(c.Limit), len(rows)))}
	if len(rows) > int(c.Limit) {
		rows = rows[:c.Limit]
		last := rows[len(rows)-1]
		pos.ID, pos.Ordering = last.PublicID, last.FirstPublishedAt.Time.UnixMicro()
		out.After = pos.encode()
	}
	for _, r := range rows {
		item, err := collectionSummary(r.PublicID, r.Slug, r.Localizations, r.FirstPublishedAt)
		if err != nil {
			return ChannelPage[CollectionSummary]{}, err
		}
		out.Items = append(out.Items, item)
	}
	return out, nil
}

func (s *Service) PublicGameCollection(ctx context.Context, market ChannelMarket, id string, c ChannelCriteria) (PublicCollection, error) {
	if !ValidPublicID(id) {
		return PublicCollection{}, ErrNotFound
	}
	if err := c.normalize("members"); err != nil {
		return PublicCollection{}, err
	}
	if s.pool == nil {
		return PublicCollection{}, ErrUnavailable
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	tx, q, err := s.channelRead(ctx)
	if err != nil {
		return PublicCollection{}, ErrUnavailable
	}
	defer tx.Rollback(context.Background())
	row, err := q.PublicGameCollection(ctx, id)
	if err != nil {
		return PublicCollection{}, classify(err)
	}
	heading, err := collectionSummary(row.PublicID, row.Slug, row.Localizations, row.FirstPublishedAt)
	if err != nil {
		return PublicCollection{}, err
	}
	members, err := channelCards(ctx, q, market, c, "members", &row.ID, id, row.Revision)
	if err != nil {
		return PublicCollection{}, err
	}
	return PublicCollection{CollectionSummary: heading, Members: members}, classify(tx.Commit(ctx))
}

func collectionRecord(ctx context.Context, q *dbgen.Queries, row dbgen.GameCollection) (CollectionRecord, error) {
	heading, err := collectionSummary(row.PublicID, row.Slug, row.Localizations, row.FirstPublishedAt)
	if err != nil {
		return CollectionRecord{}, err
	}
	members, err := q.PrivateGameCollectionMembers(ctx, row.ID)
	return CollectionRecord{CollectionSummary: heading, Status: row.Status, Revision: row.Revision, Members: members}, err
}

func (s *Service) InspectGameCollection(ctx context.Context, id string) (CollectionRecord, error) {
	if !ValidPublicID(id) {
		return CollectionRecord{}, ErrInvalid
	}
	return write(ctx, s, func(ctx context.Context, q *dbgen.Queries) (CollectionRecord, error) {
		row, err := q.LockGameCollection(ctx, id)
		if err != nil {
			return CollectionRecord{}, err
		}
		return collectionRecord(ctx, q, row)
	})
}

func (s *Service) CreateGameCollection(ctx context.Context, in Editorial) (CollectionRecord, error) {
	clean, err := normalizeEditorial(in, "")
	if err != nil {
		return CollectionRecord{}, err
	}
	text, err := json.Marshal(append([]Localization{}, clean.Localizations...))
	if err != nil {
		return CollectionRecord{}, ErrInvalid
	}
	return write(ctx, s, func(ctx context.Context, q *dbgen.Queries) (CollectionRecord, error) {
		row, err := create(s, func(id string) (dbgen.GameCollection, error) {
			return q.CreateGameCollection(ctx, dbgen.CreateGameCollectionParams{ID: uuid.NewV7(), PublicID: id, Slug: clean.Slug, Localizations: text})
		})
		if err != nil {
			return CollectionRecord{}, err
		}
		return collectionRecord(ctx, q, row)
	})
}

func (s *Service) changeCollection(ctx context.Context, id string, revision int64, change func(context.Context, *dbgen.Queries, dbgen.GameCollection) (dbgen.GameCollection, error)) (CollectionRecord, error) {
	if !ValidPublicID(id) || revision < 1 {
		return CollectionRecord{}, ErrInvalid
	}
	return write(ctx, s, func(ctx context.Context, q *dbgen.Queries) (CollectionRecord, error) {
		row, err := q.LockGameCollection(ctx, id)
		if err != nil {
			return CollectionRecord{}, err
		}
		if row.Revision != revision {
			return CollectionRecord{}, ErrConflict
		}
		row, err = change(ctx, q, row)
		if err != nil {
			return CollectionRecord{}, err
		}
		return collectionRecord(ctx, q, row)
	})
}

func (s *Service) EditGameCollection(ctx context.Context, id string, rev int64, in Editorial) (CollectionRecord, error) {
	return s.changeCollection(ctx, id, rev, func(ctx context.Context, q *dbgen.Queries, row dbgen.GameCollection) (dbgen.GameCollection, error) {
		clean, err := normalizeEditorial(in, row.Slug)
		if err != nil || (row.Status == Published && len(in.Localizations) == 0) {
			return row, ErrInvalid
		}
		text, err := json.Marshal(append([]Localization{}, clean.Localizations...))
		if err != nil {
			return row, ErrInvalid
		}
		return q.EditGameCollection(ctx, dbgen.EditGameCollectionParams{ID: row.ID, Slug: clean.Slug, Localizations: text})
	})
}

func (s *Service) SetGameCollectionMembers(ctx context.Context, id string, rev int64, ids []string) (CollectionRecord, error) {
	if len(ids) > 512 {
		return CollectionRecord{}, ErrInvalid
	}
	seen := make(map[string]bool, len(ids))
	for _, id := range ids {
		if !ValidPublicID(id) || seen[id] {
			return CollectionRecord{}, ErrInvalid
		}
		seen[id] = true
	}
	return s.changeCollection(ctx, id, rev, func(ctx context.Context, q *dbgen.Queries, row dbgen.GameCollection) (dbgen.GameCollection, error) {
		if err := q.ClearGameCollectionMembers(ctx, row.ID); err != nil {
			return row, err
		}
		count, err := q.SaveGameCollectionMembers(ctx, dbgen.SaveGameCollectionMembersParams{CollectionID: row.ID, PublicIds: ids})
		if err != nil {
			return row, err
		}
		if count != int64(len(ids)) {
			return row, ErrNotFound
		}
		return q.TouchGameCollection(ctx, row.ID)
	})
}

func (s *Service) SetGameCollectionStatus(ctx context.Context, id string, rev int64, status string) (CollectionRecord, error) {
	if status != Published && status != Archived {
		return CollectionRecord{}, ErrInvalid
	}
	return s.changeCollection(ctx, id, rev, func(ctx context.Context, q *dbgen.Queries, row dbgen.GameCollection) (dbgen.GameCollection, error) {
		if row.Status == status {
			return row, ErrConflict
		}
		if status == Published {
			info, err := collectionSummary(row.PublicID, row.Slug, row.Localizations, row.FirstPublishedAt)
			if err != nil || len(info.Localizations) == 0 {
				return row, ErrInvalid
			}
		}
		return q.UpdateGameCollectionStatus(ctx, dbgen.UpdateGameCollectionStatusParams{ID: row.ID, NewStatus: status})
	})
}

func (s *Service) GameChannelFeatures(ctx context.Context) ([]dbgen.ReadGameChannelFeaturesRow, error) {
	if s.pool == nil {
		return nil, ErrUnavailable
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	rows, err := dbgen.New(s.pool).ReadGameChannelFeatures(ctx)
	return rows, classify(err)
}

func (s *Service) SetGameChannelFeature(ctx context.Context, slot, id string, rev int64) (int64, error) {
	switch slot {
	case "primary", "secondary_1", "secondary_2", "collection_1", "collection_2", "collection_3":
	default:
		return 0, ErrInvalid
	}
	if rev < 1 || (id != "" && !ValidPublicID(id)) {
		return 0, ErrInvalid
	}
	return write(ctx, s, func(ctx context.Context, q *dbgen.Queries) (int64, error) {
		var entity *uuid.UUID
		if id != "" {
			if strings.HasPrefix(slot, "collection_") {
				row, err := q.LockGameCollection(ctx, id)
				if err != nil {
					return 0, err
				}
				entity = &row.ID
			} else {
				row, err := q.LockGamePublicID(ctx, id)
				if err != nil {
					return 0, err
				}
				entity = &row.ID
			}
		}
		next, err := q.SetGameChannelFeature(ctx, dbgen.SetGameChannelFeatureParams{Slot: slot, EntityID: entity, ExpectedRevision: rev})
		var constraint *pgconn.PgError
		if errors.Is(err, pgx.ErrNoRows) || (errors.As(err, &constraint) && constraint.Code == "23505") {
			return 0, ErrConflict
		}
		return next, err
	})
}
