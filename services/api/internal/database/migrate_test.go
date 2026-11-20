package database

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
)

func TestPostgresFailure(t *testing.T) {
	const privateDetail = "postgres://runtime:secret@example.invalid/private"
	tests := []struct {
		name string
		err  error
		want string
	}{
		{"password", &pgconn.PgError{Code: "28P01", Message: privateDetail}, "PostgreSQL password authentication failed (28P01)"},
		{"access rule", &pgconn.PgError{Code: "28000", Message: "no pg_hba.conf entry for " + privateDetail}, "PostgreSQL connection rejected by pg_hba.conf (28000)"},
		{"role", &pgconn.PgError{Code: "28000", Message: privateDetail}, "PostgreSQL role or authentication rejected (28000)"},
		{"database", &pgconn.PgError{Code: "3D000", Message: privateDetail}, "PostgreSQL database does not exist (3D000)"},
		{"privilege", &pgconn.PgError{Code: "42501", Message: privateDetail}, "PostgreSQL role has insufficient privileges (42501)"},
		{"other server error", &pgconn.PgError{Code: "XX000", Message: privateDetail}, "PostgreSQL rejected the request; inspect the server logs"},
		{"DNS", &net.DNSError{Name: privateDetail}, "PostgreSQL hostname did not resolve"},
		{"socket", &net.OpError{Op: "dial", Err: errors.New(privateDetail)}, "PostgreSQL network connection failed"},
		{"deadline", context.DeadlineExceeded, "request timed out"},
		{"cancellation", context.Canceled, "request canceled"},
		{"certificate", &tls.CertificateVerificationError{Err: errors.New(privateDetail)}, "PostgreSQL TLS negotiation or certificate verification failed"},
		{"TLS refusal", errors.New("server refused TLS connection"), "PostgreSQL TLS negotiation or certificate verification failed"},
		{"other error", errors.New(privateDetail), "unexpected database error; inspect the server logs"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := postgresFailure(fmt.Errorf("migration %s: %w", privateDetail, tc.err)); got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}
