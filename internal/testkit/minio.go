package testkit

import (
	"context"
	"testing"

	"github.com/testcontainers/testcontainers-go/modules/minio"
)

// MinIO starts MinIO and returns endpoint, keys, and a bucket name. Skips without Docker.
func MinIO(tb testing.TB) (endpoint, accessKey, secretKey, bucket string) {
	tb.Helper()
	DockerAvailable(tb)
	ctx := context.Background()
	ctr, err := minio.Run(ctx, "minio/minio:latest")
	if err != nil {
		tb.Fatalf("minio container: %v", err)
	}
	tb.Cleanup(func() {
		_ = ctr.Terminate(context.Background())
	})
	endpoint, err = ctr.ConnectionString(ctx)
	if err != nil {
		tb.Fatalf("minio endpoint: %v", err)
	}
	return endpoint, ctr.Username, ctr.Password, "artifacts"
}
