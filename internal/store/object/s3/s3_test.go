package s3

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/s3"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/mikerudolph/artifacts/internal/config"
	"github.com/mikerudolph/artifacts/internal/store/object"
	"github.com/mikerudolph/artifacts/internal/store/object/objecttest"
	"github.com/mikerudolph/artifacts/internal/testkit"
)

func TestNewRequiresBucket(t *testing.T) {
	t.Parallel()
	if _, err := New(context.Background(), config.S3{}); err == nil {
		t.Fatal("expected error")
	}
}

func TestNormalizeAndFull(t *testing.T) {
	t.Parallel()
	if got := normalizeEndpoint("http://localhost:9000"); got != "http://localhost:9000" {
		t.Fatal(got)
	}
	if got := normalizeEndpoint("localhost:9000"); got != "http://localhost:9000" {
		t.Fatal(got)
	}
	s := &store{prefix: "p"}
	if got := s.full("a/b"); got != "p/a/b" {
		t.Fatal(got)
	}
	s.prefix = ""
	if got := s.full("a/b"); got != "a/b" {
		t.Fatal(got)
	}
}

func TestIsNotFound(t *testing.T) {
	t.Parallel()
	if isNotFound(nil) || !isNotFound(&s3types.NoSuchKey{}) || !isNotFound(&s3types.NotFound{}) {
		t.Fatal("isNotFound")
	}
	if !isNotFound(errors.New("api error NotFound: blah")) {
		t.Fatal("string match")
	}
}

func TestConformance(t *testing.T) {
	base := s3Config(t)
	ctx := context.Background()
	client, err := newClient(ctx, base)
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.CreateBucket(ctx, &s3.CreateBucketInput{Bucket: &base.Bucket})
	if err != nil && !alreadyExists(err) {
		t.Fatal(err)
	}
	objecttest.Run(t, func(tb testing.TB) object.Store {
		cfg := base
		cfg.Prefix = "test-" + tb.Name()
		st, err := New(ctx, cfg)
		if err != nil {
			tb.Fatal(err)
		}
		return st
	})
}

func s3Config(t *testing.T) config.S3 {
	t.Helper()
	if bucket := os.Getenv("S3_BUCKET"); bucket != "" {
		return config.S3{
			Endpoint:     os.Getenv("S3_ENDPOINT"),
			Bucket:       bucket,
			Region:       envOr("S3_REGION", "us-east-1"),
			AccessKey:    os.Getenv("AWS_ACCESS_KEY_ID"),
			SecretKey:    os.Getenv("AWS_SECRET_ACCESS_KEY"),
			Prefix:       "test-" + t.Name(),
			UsePathStyle: true,
		}
	}
	endpoint, access, secret, _ := testkit.MinIO(t)
	return config.S3{
		Endpoint:     endpoint,
		Bucket:       "artifacts",
		Region:       "us-east-1",
		AccessKey:    access,
		SecretKey:    secret,
		Prefix:       "test-" + t.Name(),
		UsePathStyle: true,
	}
}

func envOr(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}

func alreadyExists(err error) bool {
	var e *s3types.BucketAlreadyOwnedByYou
	var e2 *s3types.BucketAlreadyExists
	return errors.As(err, &e) || errors.As(err, &e2) ||
		(err != nil && (strings.Contains(err.Error(), "BucketAlreadyOwnedByYou") || strings.Contains(err.Error(), "BucketAlreadyExists")))
}
