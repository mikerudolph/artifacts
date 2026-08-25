package s3

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
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
	plain := errors.New("plain")
	if mapErr(plain) != plain {
		t.Fatal("mapErr changed ordinary error")
	}
}

func TestImmutablePutBranches(t *testing.T) {
	t.Parallel()
	st := &store{}
	ctx := context.Background()
	if err := st.Delete(ctx, "../bad"); err == nil {
		t.Fatal("delete accepted invalid key")
	}
	if _, err := st.Exists(ctx, "../bad"); err == nil {
		t.Fatal("exists accepted invalid key")
	}
	if err := st.Copy(ctx, "../bad", "valid"); err == nil {
		t.Fatal("copy accepted invalid source")
	}
	if err := st.Copy(ctx, "valid", "../bad"); err == nil {
		t.Fatal("copy accepted invalid destination")
	}
	if _, _, _, _, err := stageUpload(strings.NewReader("x"), 2); !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("size mismatch: %v", err)
	}
	if _, _, _, _, err := stageUpload(errorReader{}, -1); err == nil {
		t.Fatal("reader error was lost")
	}
	if preconditionFailed(nil) || preconditionFailed(errors.New("ordinary")) {
		t.Fatal("ordinary error is a precondition failure")
	}
	if !preconditionFailed(errors.New("PreconditionFailed")) || !preconditionFailed(errors.New("status code: 412")) {
		t.Fatal("precondition failure not recognized")
	}
}

type errorReader struct{}

func (errorReader) Read([]byte) (int, error) { return 0, errors.New("read failed") }

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
	st, err := New(ctx, config.S3{
		Endpoint: base.Endpoint, Bucket: base.Bucket, Region: base.Region, AccessKey: base.AccessKey,
		SecretKey: base.SecretKey, Prefix: "concurrent-" + t.Name(), UsePathStyle: base.UsePathStyle,
	})
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	errs := make(chan error, 8)
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errs <- st.Put(ctx, "repo/pack/x.pack", bytes.NewBufferString("same"), 4)
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("concurrent immutable put: %v", err)
		}
	}
}

func TestLegacyObjectChecksumVerification(t *testing.T) {
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
	base.Prefix = "legacy-" + t.Name()
	raw, err := New(ctx, base)
	if err != nil {
		t.Fatal(err)
	}
	st, ok := raw.(*store)
	if !ok {
		t.Fatal("unexpected store implementation")
	}
	key := "repo/legacy.pack"
	body := []byte("legacy")
	_, err = client.PutObject(ctx, &s3.PutObjectInput{
		Bucket: &base.Bucket, Key: aws.String(st.full(key)), Body: bytes.NewReader(body), ContentLength: aws.Int64(int64(len(body))),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := st.Put(ctx, key, bytes.NewReader(body), int64(len(body))); err != nil {
		t.Fatalf("same legacy body: %v", err)
	}
	if err := st.Put(ctx, key, strings.NewReader("changed"), 7); !errors.Is(err, object.ErrImmutableConflict) {
		t.Fatalf("different legacy body: %v", err)
	}
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
