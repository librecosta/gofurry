package legacyimport

import (
	"context"
	"errors"
	"slices"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/librecosta/gofurry/services/api/internal/legacyseed"
	"github.com/librecosta/gofurry/services/api/internal/storage"
)

const (
	BootstrapAcceptance     = 1
	BootstrapInitialization = 2
)

type BootstrapInventory struct {
	SeedRows       int            `json:"seed_rows"`
	SourceExcluded int            `json:"source_excluded"`
	SourceTables   map[string]int `json:"source_tables"`
	Projection     map[string]int `json:"projection"`
}

func InspectBootstrap(ctx context.Context, seed *legacyseed.Bundle, content string, decisions []byte, version int) (BootstrapInventory, error) {
	ds, _, err := parseDecisions(decisions, content)
	if err != nil {
		return BootstrapInventory{}, err
	}
	b, err := build(ctx, seed, ds, version == BootstrapInitialization)
	if err != nil {
		return BootstrapInventory{}, err
	}
	return bootstrapInventory(b, ds, version)
}

func bootstrapInventory(b blueprint, ds []decision, version int) (BootstrapInventory, error) {
	for _, d := range ds {
		switch d.Operation {
		case "allow_publish", "link_existing", "alias_existing":
			return BootstrapInventory{}, errors.New("bootstrap package cannot include public release or existing target adoption decisions")
		}
	}
	if version == BootstrapInitialization {
		if err := checkInitializationContent(b); err != nil {
			return BootstrapInventory{}, err
		}
	}
	inventory := BootstrapInventory{SourceTables: map[string]int{}, Projection: map[string]int{}}
	for _, r := range b.Rows {
		if r.Disposition == "excluded_upstream" {
			inventory.SourceExcluded++
		} else {
			inventory.SeedRows++
			inventory.SourceTables[r.Source.Table]++
		}
	}
	totals := inventory.Projection
	logos := map[string]bool{}
	for _, it := range b.Items {
		if it.IdentityIssue != "" || it.RelationIssue != "" || it.LogoIssue == "logo_approval_pending" {
			return BootstrapInventory{}, errors.New("bootstrap package has unresolved identity, relation or ready Logo approvals")
		}
		totals[kind(it.Key.Table)]++
		totals["game_sources"] += len(it.Sources)
		totals["site_addresses"] += len(it.Addresses)
		totals["site_classified"] += flag(it.Taxonomy.Primary != "")
		for _, rel := range it.Relations {
			totals[rel.Key.Table]++
		}
		if it.Key.Table == sites {
			if it.Logo == "" {
				if version == BootstrapInitialization {
					totals["sites_using_default_logo"]++
				} else {
					totals["logo_gaps"]++
				}
			} else {
				totals["logo_references"]++
				logos[it.Logo] = true
			}
		}
	}
	totals["logo_objects"] = len(logos)
	return inventory, nil
}

func checkInitializationContent(b blueprint) error {
	for _, row := range b.Rows {
		if row.Disposition == "deferred" {
			return errors.New("bootstrap Seed has invalid source identities or unresolved references")
		}
	}
	catalogCategories := make(map[string]bool)
	for _, it := range b.Items {
		if it.Key.Table == categories {
			catalogCategories[it.Key.String()] = true
		}
	}
	for _, it := range b.Items {
		if len(it.Editorial.Localizations) == 0 {
			return errors.New("bootstrap entity requires a valid editorial name")
		}
		if it.Key.Table == tags && !catalogCategories[it.Category.String()] {
			return errors.New("bootstrap editorial tag has no mapped category")
		}
		if it.LogoIssue != "" {
			return errors.New("bootstrap has a declared Site Logo without valid bytes or binding")
		}
		if it.LifecycleIssue != "" {
			return errors.New("bootstrap has inconsistent legacy lifecycle evidence")
		}
	}
	return nil
}

func flag(value bool) int {
	if value {
		return 1
	}
	return 0
}

// RunBootstrap uses the D5-B1 engine with an empty-target or same-plan guard.
func RunBootstrap(ctx context.Context, c Config, opts Options, version int, packageSHA string) (Plan, Report, error) {
	if version == BootstrapInitialization {
		if !digestPattern.MatchString(packageSHA) {
			return Plan{}, Report{Status: "failed"}, errors.New("initialization requires a verified package SHA256")
		}
		c.bootstrapPackage = packageSHA
	} else if version != BootstrapAcceptance {
		return Plan{}, Report{Status: "failed"}, errors.New("unsupported bootstrap contract")
	}
	return run(ctx, c, opts, version)
}

// These are the base tables the acceptance import can populate. Other
// application tables must remain empty, even on retry. Foreign entities in
// this set are checked against the same-plan map before any writes.
var bootstrapTables = map[string]string{
	"sites": "site:id", "site_localizations": "site:site_id", "site_addresses": "site:site_id",
	"site_navigation_reviews": "site:site_id", "games": "game:id", "game_localizations": "game:game_id",
	"game_sources": "game:game_id", "game_collections": "collection:id", "game_collection_members": "collection:collection_id,game:game_id",
	"game_collection_tag_rules": "collection:collection_id,tag:tag_id", "game_collection_exclusions": "collection:collection_id,game:game_id",
	"game_editorial_tags": "tag:id,tag_category:category_id", "game_editorial_tag_categories": "tag_category:id",
	"game_editorial_tag_members": "game:game_id,tag:tag_id",
}

var bootstrapWrites = map[string]string{
	"sites": "INSERT,UPDATE", "site_localizations": "INSERT,UPDATE,DELETE", "site_addresses": "INSERT,UPDATE",
	"site_navigation_reviews": "INSERT", "games": "INSERT,UPDATE", "game_localizations": "INSERT,UPDATE,DELETE", "game_sources": "INSERT,UPDATE",
	"game_collections": "INSERT,UPDATE", "game_collection_members": "INSERT,DELETE",
	"game_collection_tag_rules": "INSERT,DELETE", "game_collection_exclusions": "INSERT,DELETE",
	"game_editorial_tags": "INSERT,UPDATE", "game_editorial_tag_categories": "INSERT,UPDATE", "game_editorial_tag_members": "INSERT",
	"catalog_publication_decisions": "INSERT", "legacy_import_batches": "INSERT", "legacy_import_receipts": "INSERT",
	"legacy_site_ids": "INSERT", "legacy_game_ids": "INSERT", "legacy_editorial_ids": "INSERT",
}

func bootstrapTarget(ctx context.Context, conn *pgx.Conn, mode, planSHA string, p Plan) error {
	tx, err := conn.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return errors.New("cannot inspect bootstrap target")
	}
	defer tx.Rollback(ctx)
	var otherBatch, sameBatch, seeded bool
	err = tx.QueryRow(ctx, `SELECT
		EXISTS (SELECT FROM public.legacy_import_batches WHERE plan_sha256<>$1),
		EXISTS (SELECT FROM public.legacy_import_batches WHERE plan_sha256=$1 AND backup_sha256=$2 AND content_sha256=$3 AND decisions_sha256=$4),
		(SELECT count(*)=3 AND bool_and(revision=0 AND site_id IS NULL) FROM public.site_navigation_features)
		  AND (SELECT count(*)=6 AND bool_and(revision=1 AND entity_id IS NULL) FROM public.game_channel_features)`,
		planSHA, p.Backup, p.Content, p.Decisions).Scan(&otherBatch, &sameBatch, &seeded)
	if err != nil {
		return errors.New("cannot read bootstrap target state or database permissions")
	}
	if otherBatch || !seeded || mode == "verify" && !sameBatch {
		return errors.New("bootstrap requires a fresh database or its exact registered plan")
	}

	var tables []string
	rows, err := tx.Query(ctx, `SELECT c.relname::text FROM pg_catalog.pg_class c JOIN pg_catalog.pg_namespace n ON n.oid=c.relnamespace
		WHERE n.nspname='public' AND c.relkind IN ('r','p') ORDER BY c.relname`)
	if err != nil {
		return errors.New("cannot inspect bootstrap table inventory")
	}
	tables, err = pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		return errors.New("cannot inspect bootstrap table inventory")
	}
	if mode != "verify" {
		for name, permissions := range bootstrapWrites {
			for _, permission := range strings.Split(permissions, ",") {
				var permitted bool
				if tx.QueryRow(ctx, "SELECT has_table_privilege(current_user,$1,$2)", "public."+name, permission).Scan(&permitted) != nil || !permitted {
					return errors.New("bootstrap database role lacks required write privileges")
				}
			}
		}
	}
	for _, table := range tables {
		if slices.Contains([]string{"goose_db_version", "site_navigation_features", "game_channel_features"}, table) {
			continue
		}
		where := "true"
		if sameBatch {
			where = bootstrapForeignRow(table)
		}
		var exists bool
		if err = tx.QueryRow(ctx, "SELECT EXISTS(SELECT FROM "+pgx.Identifier{"public", table}.Sanitize()+" t WHERE "+where+")", pgx.NamedArgs{"plan": planSHA}).Scan(&exists); err != nil {
			return errors.New("cannot inspect bootstrap target data or database permissions")
		}
		if exists {
			return errors.New("bootstrap target contains unrelated data")
		}
	}
	if tx.Commit(ctx) != nil {
		return errors.New("cannot complete bootstrap target preflight")
	}
	return nil
}

func bootstrapForeignRow(table string) string {
	switch table {
	case "legacy_site_ids", "legacy_game_ids", "legacy_editorial_ids":
		return "t.first_plan_sha256<>@plan OR t.origin<>'created'"
	case "legacy_import_batches", "legacy_import_receipts":
		return "t.plan_sha256<>@plan"
	case "catalog_publication_decisions":
		return "t.actor<>'legacy_import' OR NOT EXISTS (SELECT FROM public.legacy_editorial_mappings m WHERE m.target_id=t.entity_id AND m.kind=t.entity_kind AND m.first_plan_sha256=@plan)"
	}
	fields, ok := bootstrapTables[table]
	if !ok {
		return "true"
	}
	conditions := []string{}
	for _, field := range strings.Split(fields, ",") {
		kind, column, _ := strings.Cut(field, ":")
		conditions = append(conditions, "NOT EXISTS (SELECT FROM public.legacy_editorial_mappings m WHERE m.kind='"+kind+"' AND m.target_id=t."+column+" AND m.first_plan_sha256=@plan)")
	}
	return strings.Join(conditions, " OR ")
}

func bootstrapStorage(ctx context.Context, objects *storage.Store, b blueprint) error {
	index := slices.IndexFunc(b.Items, func(it intent) bool { return it.Logo != "" })
	if index == -1 {
		return nil
	}
	if objects == nil {
		return errors.New("bootstrap requires isolated object storage for packaged Logos")
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	return objects.CheckSiteLogoDestination(ctx, logoKey(b.Items[index].Logo))
}
