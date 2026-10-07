package postgres

import (
	"context"
	"log/slog"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/feature/rds/auth"
	"github.com/jackc/pgx/v5"
	"github.com/mikerudolph/artifacts/internal/config"
)

var warnIAMTLS = sync.OnceFunc(func() {
	slog.Warn("RDS IAM TLS is configured without full server identity verification; use sslmode=verify-full with a trusted CA")
})

func databaseIAM(ctx context.Context, cfg config.Postgres, conn *pgx.ConnConfig) (func(context.Context, *pgx.ConnConfig) error, error) {
	if conn.Host == "" || strings.HasPrefix(conn.Host, "/") || net.ParseIP(conn.Host) != nil || conn.User == "" || len(conn.Fallbacks) != 0 {
		return nil, setupFailure("RDS IAM requires one DNS endpoint and a database username, without connection fallbacks")
	}
	if conn.Password != "" {
		return nil, setupFailure("RDS IAM requires a connection without a static database password")
	}
	if err := requireIAMTLS(cfg.DSN, conn); err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	options := []func(*awsconfig.LoadOptions) error{}
	if cfg.Region != "" {
		options = append(options, awsconfig.WithRegion(cfg.Region))
	}
	awsCfg, err := awsconfig.LoadDefaultConfig(ctx, options...)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, setupFailure("cannot load AWS configuration for database authentication")
	}
	if awsCfg.Region == "" {
		return nil, setupFailure("RDS IAM requires ARTIFACTS_DATABASE_REGION or an AWS SDK region")
	}
	if conn.TLSConfig.InsecureSkipVerify || conn.TLSConfig.ServerName != conn.Host {
		warnIAMTLS()
	}
	return iamBeforeConnect(awsCfg.Region, awsCfg.Credentials), nil
}

func iamBeforeConnect(region string, credentials aws.CredentialsProvider) func(context.Context, *pgx.ConnConfig) error {
	return func(ctx context.Context, conn *pgx.ConnConfig) error {
		ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
		defer cancel()
		if ctx.Err() != nil {
			return ctx.Err()
		}
		endpoint := net.JoinHostPort(conn.Host, strconv.Itoa(int(conn.Port)))
		token, err := auth.BuildAuthToken(ctx, endpoint, region, conn.User, credentials)
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			return setupFailure("cannot obtain AWS credentials or sign an RDS IAM login token")
		}
		conn.Password = token
		return nil
	}
}
