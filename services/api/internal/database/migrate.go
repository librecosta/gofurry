package database

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"time"

	"github.com/jackc/pgx/v5/stdlib"
	"github.com/librecosta/gofurry/services/api/internal/platform"
	"github.com/pressly/goose/v3"
)

func Migrate(ctx context.Context, cfg platform.DatabaseConfig, dir, action string, out io.Writer) error {
	if action != "status" && action != "up" {
		return errors.New("unsupported migration action")
	}
	files := os.DirFS(dir)
	names, err := fs.Glob(files, "*.sql")
	if err != nil {
		return errors.New("cannot inspect migration directory")
	}
	if _, err := fs.Stat(files, "."); err != nil {
		return errors.New("migration directory does not exist")
	}
	// No provider or version table is created until a real SQL migration exists.
	if len(names) == 0 {
		fmt.Fprintln(out, "No SQL migrations; database untouched")
		return nil
	}
	pc, err := PoolConfig(cfg)
	if err != nil {
		return err
	}
	db := stdlib.OpenDB(*pc.ConnConfig)
	db.SetMaxOpenConns(1)
	defer db.Close()
	probe, cancel := context.WithTimeout(ctx, cfg.ConnectTimeout)
	defer cancel()
	if db.PingContext(probe) != nil {
		return errors.New("migration database connection failed")
	}
	provider, err := goose.NewProvider(goose.DialectPostgres, db, files, goose.WithDisableGlobalRegistry(true))
	if err != nil {
		return errors.New("invalid migration files")
	}
	bounded, stop := context.WithTimeout(ctx, time.Minute)
	defer stop()
	switch action {
	case "up":
		var results []*goose.MigrationResult
		results, err = provider.Up(bounded)
		if err == nil {
			fmt.Fprintf(out, "Applied %d migrations\n", len(results))
		}
	case "status":
		// Goose Status initializes its version table. Status must never write:
		// inspect only metadata inside a PostgreSQL read-only transaction.
		var tx *sql.Tx
		tx, err = db.BeginTx(bounded, &sql.TxOptions{ReadOnly: true})
		if err != nil {
			break
		}
		defer tx.Rollback()
		var exists bool
		if err = tx.QueryRowContext(bounded, "SELECT to_regclass('public.goose_db_version') IS NOT NULL").Scan(&exists); err != nil {
			break
		}
		applied := map[int64]bool{}
		if exists {
			var rows *sql.Rows
			rows, err = tx.QueryContext(bounded, "SELECT version_id,is_applied FROM public.goose_db_version ORDER BY id")
			if err != nil {
				break
			}
			for rows.Next() {
				var version int64
				var yes bool
				if err = rows.Scan(&version, &yes); err != nil {
					break
				}
				applied[version] = yes
			}
			if err == nil {
				err = rows.Err()
			}
			rows.Close()
		}
		if err == nil {
			for _, source := range provider.ListSources() {
				state := "pending"
				if applied[source.Version] {
					state = "applied"
				}
				fmt.Fprintf(out, "%d %s\n", source.Version, state)
			}
		}
	default:
		return errors.New("unsupported migration action")
	}
	if err != nil {
		return errors.New("database migration operation failed")
	}
	return nil
}
