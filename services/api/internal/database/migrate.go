package database

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net"
	"os"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
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
	if err := db.PingContext(probe); err != nil {
		return fmt.Errorf("migration database connection failed: %s", postgresFailure(err))
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
		return fmt.Errorf("database migration operation failed: %s", postgresFailure(err))
	}
	return nil
}

// Connection and Goose errors can include credentials, paths or SQL. Keep only
// a fixed diagnostic selected from the underlying error.
func postgresFailure(err error) string {
	var server *pgconn.PgError
	if errors.As(err, &server) {
		switch server.Code {
		case "28P01":
			return "PostgreSQL password authentication failed (28P01)"
		case "28000":
			if strings.Contains(server.Message, "pg_hba.conf") {
				return "PostgreSQL connection rejected by pg_hba.conf (28000)"
			}
			return "PostgreSQL role or authentication rejected (28000)"
		case "3D000":
			return "PostgreSQL database does not exist (3D000)"
		case "42501":
			return "PostgreSQL role has insufficient privileges (42501)"
		default:
			return "PostgreSQL rejected the request; inspect the server logs"
		}
	}
	var dns *net.DNSError
	var socket *net.OpError
	var certificate *tls.CertificateVerificationError
	var authority x509.UnknownAuthorityError
	var hostname x509.HostnameError
	var invalidCertificate x509.CertificateInvalidError
	var record tls.RecordHeaderError
	var alert tls.AlertError
	switch {
	case errors.Is(err, context.Canceled):
		return "request canceled"
	case errors.Is(err, context.DeadlineExceeded), pgconn.Timeout(err), errors.As(err, &socket) && socket.Timeout():
		return "request timed out"
	case errors.As(err, &dns):
		return "PostgreSQL hostname did not resolve"
	case errors.As(err, &certificate), errors.As(err, &authority), errors.As(err, &hostname), errors.As(err, &invalidCertificate),
		errors.As(err, &record), errors.As(err, &alert), strings.Contains(err.Error(), "server refused TLS connection"):
		return "PostgreSQL TLS negotiation or certificate verification failed"
	case errors.As(err, &socket):
		return "PostgreSQL network connection failed"
	default:
		return "unexpected database error; inspect the server logs"
	}
}
