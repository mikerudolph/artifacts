package s3

import (
	"context"
	"errors"
	"fmt"
	"io"
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

// New returns an S3-compatible object.Store.
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
	in := &s3.PutObjectInput{Bucket: &s.bucket, Key: aws.String(s.full(key)), Body: r}
	if size >= 0 {
		in.ContentLength = aws.Int64(size)
	}
	_, err := s.client.PutObject(ctx, in)
	return err
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
	ok, err := s.Exists(ctx, src)
	if err != nil {
		return err
	}
	if !ok {
		return object.ErrNotFound
	}
	srcPath := path.Join(s.bucket, s.full(src))
	_, err = s.client.CopyObject(ctx, &s3.CopyObjectInput{
		Bucket:     &s.bucket,
		Key:        aws.String(s.full(dst)),
		CopySource: aws.String(srcPath),
	})
	return err
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
