package s3

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/mikerudolph/artifacts/internal/config"
	"github.com/mikerudolph/artifacts/internal/store/object"
)

type store struct {
	client *s3.Client
	bucket string
	prefix string
}

func New(ctx context.Context, cfg config.S3) (object.Store, error) {
	if cfg.Bucket == "" {
		return nil, fmt.Errorf("S3 bucket is required")
	}
	client, err := newClient(ctx, cfg)
	if err != nil {
		return nil, err
	}
	return &store{client: client, bucket: cfg.Bucket, prefix: strings.Trim(cfg.Prefix, "/")}, nil
}

func newClient(ctx context.Context, cfg config.S3) (*s3.Client, error) {
	var opts []func(*awsconfig.LoadOptions) error
	if cfg.Region != "" {
		opts = append(opts, awsconfig.WithRegion(cfg.Region))
	}
	if cfg.AccessKey != "" {
		opts = append(opts, awsconfig.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider(cfg.AccessKey, cfg.SecretKey, ""),
		))
	}
	awsCfg, err := awsconfig.LoadDefaultConfig(ctx, opts...)
	if err != nil {
		return nil, err
	}
	return s3.NewFromConfig(awsCfg, func(o *s3.Options) {
		if cfg.Endpoint != "" {
			o.BaseEndpoint = aws.String(normalizeEndpoint(cfg.Endpoint))
		}
		o.UsePathStyle = cfg.UsePathStyle
	}), nil
}

func normalizeEndpoint(ep string) string {
	if strings.HasPrefix(ep, "http://") || strings.HasPrefix(ep, "https://") {
		return ep
	}
	return "http://" + ep
}

func (s *store) full(key string) string {
	if s.prefix == "" {
		return key
	}
	return path.Join(s.prefix, key)
}

func (s *store) Get(ctx context.Context, key string) (io.ReadCloser, error) {
	if err := object.ValidateKey(key); err != nil {
		return nil, err
	}
	out, err := s.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: &s.bucket,
		Key:    aws.String(s.full(key)),
	})
	if err != nil {
		return nil, mapErr(err)
	}
	return out.Body, nil
}

func (s *store) Put(ctx context.Context, key string, r io.Reader, size int64) error {
	if err := object.ValidateKey(key); err != nil {
		return err
	}
	body, checksum, actualSize, cleanup, err := stageUpload(r, size)
	if err != nil {
		return err
	}
	defer cleanup()
	if same, exists, err := s.matches(ctx, key, checksum, actualSize); err != nil {
		return err
	} else if exists {
		if same {
			return nil
		}
		return object.ErrImmutableConflict
	}
	in := &s3.PutObjectInput{
		Bucket: &s.bucket, Key: aws.String(s.full(key)), Body: body, IfNoneMatch: aws.String("*"),
		ContentLength: aws.Int64(actualSize), ChecksumSHA256: aws.String(base64.StdEncoding.EncodeToString(checksum)),
		Metadata: map[string]string{"sha256": hex.EncodeToString(checksum)},
	}
	_, err = s.client.PutObject(ctx, in)
	if err != nil && preconditionFailed(err) {
		same, _, verifyErr := s.matches(ctx, key, checksum, actualSize)
		if verifyErr != nil {
			return verifyErr
		}
		if same {
			return nil
		}
		return object.ErrImmutableConflict
	}
	return err
}

func stageUpload(r io.Reader, expected int64) (*os.File, []byte, int64, func(), error) {
	f, err := os.CreateTemp("", "artifacts-s3-*")
	if err != nil {
		return nil, nil, 0, func() {}, err
	}
	cleanup := func() { _ = os.Remove(f.Name()); _ = f.Close() }
	h := sha256.New()
	n, err := io.Copy(io.MultiWriter(f, h), r)
	if err != nil || (expected >= 0 && n != expected) {
		if err == nil {
			err = io.ErrUnexpectedEOF
		}
		cleanup()
		return nil, nil, 0, func() {}, err
	}
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		cleanup()
		return nil, nil, 0, func() {}, err
	}
	return f, h.Sum(nil), n, cleanup, nil
}

func (s *store) matches(ctx context.Context, key string, checksum []byte, size int64) (bool, bool, error) {
	head, err := s.client.HeadObject(ctx, &s3.HeadObjectInput{Bucket: &s.bucket, Key: aws.String(s.full(key))})
	if err != nil {
		if isNotFound(err) {
			return false, false, nil
		}
		return false, false, err
	}
	if head.ContentLength == nil || *head.ContentLength != size {
		return false, true, nil
	}
	want := hex.EncodeToString(checksum)
	if head.Metadata["sha256"] != "" {
		return head.Metadata["sha256"] == want, true, nil
	}
	r, err := s.Get(ctx, key)
	if err != nil {
		return false, true, err
	}
	defer func() { _ = r.Close() }()
	h := sha256.New()
	_, err = io.Copy(h, r)
	return err == nil && hex.EncodeToString(h.Sum(nil)) == want, true, err
}

func preconditionFailed(err error) bool {
	return err != nil && (strings.Contains(err.Error(), "PreconditionFailed") || strings.Contains(err.Error(), "status code: 412"))
}

func (s *store) Delete(ctx context.Context, key string) error {
	if err := object.ValidateKey(key); err != nil {
		return err
	}
	_, err := s.client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: &s.bucket,
		Key:    aws.String(s.full(key)),
	})
	return err
}

func (s *store) Exists(ctx context.Context, key string) (bool, error) {
	if err := object.ValidateKey(key); err != nil {
		return false, err
	}
	_, err := s.client.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: &s.bucket,
		Key:    aws.String(s.full(key)),
	})
	if err != nil {
		if isNotFound(err) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func (s *store) Copy(ctx context.Context, src, dst string) error {
	if err := object.ValidateKey(src); err != nil {
		return err
	}
	if err := object.ValidateKey(dst); err != nil {
		return err
	}
	r, err := s.Get(ctx, src)
	if err != nil {
		return err
	}
	defer func() { _ = r.Close() }()
	return s.Put(ctx, dst, r, -1)
}

func mapErr(err error) error {
	if isNotFound(err) {
		return object.ErrNotFound
	}
	return err
}

func isNotFound(err error) bool {
	if err == nil {
		return false
	}
	var nsk *s3types.NoSuchKey
	var nf *s3types.NotFound
	if errors.As(err, &nsk) || errors.As(err, &nf) {
		return true
	}
	msg := err.Error()
	return strings.Contains(msg, "NoSuchKey") || strings.Contains(msg, "NotFound")
}
