package legacyseed

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"
)

const rowLimit = 250_000

type table struct {
	db, name, file, keys, cols string
	filter, extra              string
}

const editorial = "name:s name_en:s info:s info_en:s"
const oldTimes = "create_time:l update_time:l"
const times = "created_at:z updated_at:z"
const addresses = "site_addresses_candidates"

var tables = []table{
	{"gfn", "gfn_site", "sites", "id", "id:i " + editorial + " " + oldTimes + " nsfw:s icon:s deleted:b deleted_at:z deletion_reason_code:s deletion_reason:s", "", ""},
	{"gfn", "gfn_site_group", "site_groups", "id", "id:i " + editorial + " priority:i " + oldTimes, "", "count(*) OVER (PARTITION BY lower(btrim(t.name))) AS zh_name_count, count(*) OVER (PARTITION BY lower(btrim(t.name_en))) AS en_name_count"},
	{"gfn", "gfn_site_group_map", "site_group_members", "id", "id:i site_id:i group_id:i weight:i " + oldTimes, "", "count(*) OVER (PARTITION BY t.site_id,t.group_id) AS pair_count"},
	{"gfn", "gfn_collector_domain", addresses, "id", "id:i name:s prefix:s tls:s site_id:i deleted:b", "", ""},
	{"gfn", "gfn_target_tracking_periods", addresses, "id", "id:i collector_domain_id:i site_id:i target:s tracked_from:z tracked_until:z tracking_basis:s opened_reason:s closed_reason:s " + times, "", ""},
	{"gfn", "gfn_site_primary_target_periods", addresses, "id", "id:i site_id:i target_tracking_period_id:i effective_from:z effective_until:z basis:s " + times, "", ""},
	{"gfn", "gfn_featured_site", "site_featured_candidates", "id", "id:i site_id:i weight:i " + oldTimes, "", "count(*) OVER (PARTITION BY t.site_id) AS featured_count"},
	{"gfg", "gfg_game", "games", "id", "id:i " + editorial + " " + oldTimes + " resources:j groups:j links:j developers:j publishers:j release_date:s appid:i weight:i showcase_eligible:b", "", "count(*) OVER (PARTITION BY t.appid) AS appid_count, EXISTS(SELECT 1 FROM public.gfg_game_tag gt JOIN public.gfg_tag tag ON tag.id=gt.tag_id WHERE gt.game_id=t.id AND tag.code='adult') AS legacy_adult"},
	{"gfg", "gfg_tag_category", "game_tag_categories", "id", "id:i code:s " + editorial + " sort_order:n archived_at:l " + oldTimes, "", "count(*) OVER (PARTITION BY t.code) AS code_count"},
	{"gfg", "gfg_tag", "game_tags", "id", "id:i code:s " + editorial + " category_id:i archived_at:l " + oldTimes, "", "count(*) OVER (PARTITION BY t.code) AS code_count"},
	{"gfg", "gfg_game_tag", "game_tag_members", "game_id,tag_id", "game_id:i tag_id:i role:s " + oldTimes, "", "count(*) OVER (PARTITION BY t.game_id,t.role) AS role_count"},
	{"gfg", "gfg_game_collection", "game_collections", "id", "id:i code:s " + editorial + " status:s version:i published_at:z archived_at:z " + times, "", "count(*) OVER (PARTITION BY t.code) AS code_count"},
	{"gfg", "gfg_game_collection_item", "collection_manual_members", "collection_id,game_id", "collection_id:i game_id:i " + times, "", "count(*) OVER (PARTITION BY t.collection_id) AS member_count, row_number() OVER (PARTITION BY t.collection_id ORDER BY t.game_id) AS export_order, EXISTS(SELECT 1 FROM public.gfg_game_collection_exclusion x WHERE x.collection_id=t.collection_id AND x.game_id=t.game_id) AS explicitly_excluded"},
	{"gfg", "gfg_game_collection_tag", "collection_tag_rules", "collection_id,tag_id", "collection_id:i tag_id:i created_at:z", "", ""},
	{"gfg", "gfg_game_collection_exclusion", "collection_exclusions", "collection_id,game_id", "collection_id:i game_id:i created_at:z", "", ""},
	{"gfg", "gfg_game_collection_home_slot", "collection_home_slots", "slot", "slot:h collection_id:i " + times, "", "count(*) OVER (PARTITION BY t.collection_id) AS slot_count"},
	{"gfg", "gfg_game_first_available", "game_release_candidates", "game_id", "game_id:i precision:s exact_date:d release_year:n release_month:n release_quarter:n window_start:d window_end:d source:s inferred:b source_raw:s source_observed_at:z normalizer_version:s established_at:z updated_at:z", "source = 'legacy_manual' AND inferred = false", ""},
	{"gfg", "gfg_game_release_state", "game_release_candidates", "game_id", "game_id:i availability:s precision:s exact_date:d release_year:n release_month:n release_quarter:n window_start:d window_end:d raw_text:s source:s source_region:s source_locale:s normalizer_version:s observed_at:z updated_at:z", "false", ""},
}

var typeNames = map[string][]string{
	"i": {"int8"}, "n": {"int4"}, "h": {"int2"}, "s": {"text", "varchar", "bpchar"}, "j": {"jsonb"},
	"l": {"timestamp"}, "z": {"timestamptz"}, "d": {"date"}, "b": {"bool"},
}

func (s table) fields() []string { return strings.Fields(s.cols) }
func (s table) path() string     { return "content/" + s.file + ".ndjson" }

func (s table) query() string {
	// Each column is a compiled-in legacy identifier. Large source values are
	// represented by an explicit marker before they cross the PostgreSQL wire.
	var columns []string
	for _, field := range s.fields() {
		name, typ, _ := strings.Cut(field, ":")
		expr := "t." + name
		if typ == "s" || typ == "j" {
			expr = "CASE WHEN pg_catalog.octet_length(" + expr + "::text) > 8192 THEN '{\"source_value_oversize\":true}'::jsonb ELSE pg_catalog.to_jsonb(" + expr + ") END AS " + name
		}
		columns = append(columns, expr)
	}
	if s.extra != "" {
		columns = append(columns, s.extra)
	}
	q := "SELECT pg_catalog.to_jsonb(seed) FROM (SELECT " + strings.Join(columns, ",") + " FROM public." + s.name + " t"
	if s.filter != "" {
		q += " WHERE " + s.filter
	}
	return q + ") seed ORDER BY " + s.keys
}

func openSnapshot(ctx context.Context, db Database) (*pgx.Conn, pgx.Tx, error) {
	for _, item := range os.Environ() {
		name, _, _ := strings.Cut(item, "=")
		if strings.HasPrefix(strings.ToUpper(name), "PG") {
			return nil, nil, errors.New("remove PG* environment settings before opening a legacy snapshot")
		}
	}
	dsn := url.URL{Scheme: "postgres", Host: db.Host, Path: "/" + db.Name, User: url.UserPassword(db.User, db.Password)}
	if strings.Contains(db.Host, ":") {
		dsn.Host = "[" + db.Host + "]"
	}
	dsn.Host += ":" + strconv.Itoa(int(db.Port))
	dsn.RawQuery = "sslmode=disable&connect_timeout=5"
	c, err := pgx.ParseConfig(dsn.String())
	if err != nil {
		return nil, nil, errors.New("invalid source connection settings")
	}
	c.RuntimeParams = map[string]string{
		"application_name": "gofurry-legacy-seed", "search_path": "pg_catalog", "TimeZone": "UTC",
		"statement_timeout": "30000", "lock_timeout": "3000", "idle_in_transaction_session_timeout": "960000",
		"default_transaction_read_only": "on", "client_encoding": "UTF8",
	}

	conn, err := pgx.ConnectConfig(ctx, c)
	if err != nil {
		return nil, nil, errors.New("cannot connect to isolated legacy database")
	}
	fail := func() (*pgx.Conn, pgx.Tx, error) {
		closeConn(conn)
		return nil, nil, errors.New("legacy source requires PostgreSQL 18 UTF8, a restricted reader and read-only snapshot")
	}
	tx, err := conn.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return fail()
	}
	var valid bool
	err = tx.QueryRow(ctx, `SELECT current_database()=$1 AND current_user=$2
		AND current_setting('server_version_num')::integer BETWEEN 180000 AND 189999
		AND current_setting('server_encoding')='UTF8'
		AND current_setting('transaction_read_only')='on'
		AND current_setting('transaction_isolation')='repeatable read'
		AND NOT (rolsuper OR rolcreatedb OR rolcreaterole OR rolreplication OR rolbypassrls
			OR EXISTS(SELECT FROM pg_auth_members WHERE member=r.oid)
			OR has_database_privilege(current_database(),'CREATE')
			OR has_schema_privilege('public','CREATE'))
		FROM pg_roles r WHERE rolname=current_user`, db.Name, db.User).Scan(&valid)
	if err != nil || !valid {
		return fail()
	}
	return conn, tx, nil
}

func closeConn(conn *pgx.Conn) {
	_ = conn.Close(context.Background())
}

func checkTable(ctx context.Context, tx pgx.Tx, s table) error {
	rows, err := tx.Query(ctx, `SELECT a.attname,t.typname,has_table_privilege(c.oid,'SELECT'),
		has_table_privilege(c.oid,'INSERT,UPDATE,DELETE,TRUNCATE,TRIGGER,REFERENCES,MAINTAIN')
		OR has_any_column_privilege(c.oid,'INSERT,UPDATE,REFERENCES')
		OR c.relowner=(SELECT oid FROM pg_roles WHERE rolname=current_user)
		FROM pg_class c JOIN pg_namespace n ON c.relnamespace=n.oid
		JOIN pg_attribute a ON a.attrelid=c.oid JOIN pg_type t ON t.oid=a.atttypid
		WHERE n.nspname='public' AND c.relname=$1 AND c.relkind='r' AND NOT c.relrowsecurity
		AND a.attnum>0 AND NOT a.attisdropped`, s.name)
	if err != nil {
		return fmt.Errorf("%s: cannot verify source schema", s.name)
	}
	defer rows.Close()
	found := map[string]string{}
	for rows.Next() {
		var field, typ string
		var canRead, canWrite bool
		if rows.Scan(&field, &typ, &canRead, &canWrite) != nil || !canRead || canWrite {
			return fmt.Errorf("%s: requires table SELECT and no write or owner privilege", s.name)
		}
		found[field] = typ
	}
	if rows.Err() != nil {
		return fmt.Errorf("%s: cannot read source schema", s.name)
	}
	for _, field := range s.fields() {
		name, typ, _ := strings.Cut(field, ":")
		ok := false
		for _, expected := range typeNames[typ] {
			ok = ok || found[name] == expected
		}
		if !ok {
			return fmt.Errorf("%s.%s: missing or incompatible legacy source column", s.name, name)
		}
	}
	return nil
}

func canReadIdeas(ctx context.Context, tx pgx.Tx) bool {
	var readOnly bool
	err := tx.QueryRow(ctx, `SELECT has_column_privilege(c.oid,a.attnum,'SELECT')
		AND NOT has_table_privilege(c.oid,'SELECT,INSERT,UPDATE,DELETE,TRUNCATE,TRIGGER,REFERENCES,MAINTAIN')
		AND NOT has_any_column_privilege(c.oid,'INSERT,UPDATE,REFERENCES')
		AND NOT EXISTS(SELECT FROM pg_attribute private_col WHERE private_col.attrelid=c.oid
			AND private_col.attnum>0 AND NOT private_col.attisdropped AND private_col.attname<>'kind'
			AND has_column_privilege(c.oid,private_col.attnum,'SELECT'))
		FROM pg_class c JOIN pg_namespace n ON n.oid=c.relnamespace
		JOIN pg_attribute a ON a.attrelid=c.oid JOIN pg_type t ON t.oid=a.atttypid
		WHERE n.nspname='public' AND c.relname='gfa_content_idea' AND c.relkind='r' AND NOT c.relrowsecurity
		AND a.attname='kind' AND NOT a.attisdropped AND t.typname IN ('text','varchar')`).Scan(&readOnly)
	return err == nil && readOnly
}
