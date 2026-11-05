package database

import (
	"context"
	"errors"
	"net"
	"net/url"
	"os"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/librecosta/gofurry/services/api/internal/platform"
)

func PoolConfig(c platform.DatabaseConfig) (*pgxpool.Config, error) {
	// pgx internally honors libpq environment variables. Reject these rather than
	// silently inheriting a service file, password file, connection or SQL options.
	for _, entry := range os.Environ() {
		key, _, _ := strings.Cut(entry, "=")
		if strings.HasPrefix(strings.ToUpper(key), "PG") {
			return nil, errors.New("PG* environment variables are not supported; use only the explicit YAML")
		}
	}
	u := url.URL{Scheme: "postgres", Host: net.JoinHostPort(c.Host, strconv.Itoa(int(c.Port))), Path: "/" + c.Name, User: url.UserPassword(c.User, "unused-parser-password")}
	q := url.Values{"sslmode": {c.SSLMode}, "sslcert": {""}, "sslkey": {""}, "sslrootcert": {""}, "application_name": {"gofurry"}}
	u.RawQuery = q.Encode()
	pc, err := pgxpool.ParseConfig(u.String())
	if err != nil {
		return nil, errors.New("invalid database configuration")
	}
	pc.ConnConfig.Password = c.Password
	pc.ConnConfig.ConnectTimeout = c.ConnectTimeout
	pc.MaxConns, pc.MinConns = c.MaxConnections, c.MinConnections
	pc.MaxConnLifetime, pc.MaxConnIdleTime = c.MaxConnectionLifetime, c.MaxConnectionIdleTime
	return pc, nil
}

func Open(ctx context.Context, c platform.DatabaseConfig) (*pgxpool.Pool, error) {
	pc, err := PoolConfig(c)
	if err != nil {
		return nil, err
	}
	pool, err := pgxpool.NewWithConfig(ctx, pc)
	if err != nil {
		return nil, errors.New("database pool initialization failed")
	}
	// Optional dependency: retain a reconnectable pool during an outage. The
	// startup owner probes it; wallpaper requests fail closed independently.
	return pool, nil
}
