package testkit

import (
	"context"
	"testing"

	"github.com/testcontainers/testcontainers-go/modules/minio"
)

func MinIO(tb testing.TB) (endpoint, accessKey, secretKey, bucket string) {
	tb.Helper()
	DockerAvailable(tb)
	ctx := context.Background()
	ctr, err := minio.Run(ctx, "quay.io/minio/minio:RELEASE.2025-09-07T16-13-09Z@sha256:14cea493d9a34af32f524e538b8346cf79f3321eff8e708c1e2960462bd8936e")
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
