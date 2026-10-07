package postgres

import (
	"net/url"
	"strings"

	"github.com/jackc/pgx/v5"
)

func requireIAMTLS(dsn string, conn *pgx.ConnConfig) error {
	if strings.HasPrefix(dsn, "postgres://") || strings.HasPrefix(dsn, "postgresql://") {
		u, err := url.Parse(dsn)
		if err != nil {
			return setupFailure("invalid database connection configuration")
		}
		query := u.Query()
		query.Set("sslrootcert", "")
		query.Set("sslnegotiation", "postgres")
		u.RawQuery = query.Encode()
		dsn = u.String()
	} else {
		dsn += " sslrootcert='' sslnegotiation=postgres"
	}
	policy, err := pgx.ParseConfig(dsn)
	if err != nil || policy.TLSConfig == nil || len(policy.Fallbacks) != 0 || conn.TLSConfig == nil {
		return setupFailure("RDS IAM requires sslmode=require, verify-ca, or verify-full without plaintext fallback")
	}
	return nil
}
