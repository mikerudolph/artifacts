package s3

import (
	"context"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/mikerudolph/artifacts/internal/store/object"
)

func (s *store) List(ctx context.Context, prefix string) ([]string, error) {
	full := s.full(prefix)
	var keys []string
	p := s3.NewListObjectsV2Paginator(s.client, &s3.ListObjectsV2Input{
		Bucket: &s.bucket,
		Prefix: aws.String(full),
	})
	for p.HasMorePages() {
		page, err := p.NextPage(ctx)
		if err != nil {
			return nil, err
		}
		for _, obj := range page.Contents {
			if obj.Key == nil {
				continue
			}
			key := strings.TrimPrefix(*obj.Key, s.prefix)
			key = strings.TrimPrefix(key, "/")
			keys = append(keys, key)
		}
	}
	return keys, nil
}

func (s *store) DeletePrefix(ctx context.Context, prefix string) error {
	if prefix != "" {
		if err := object.ValidateKey(prefix); err != nil {
			return err
		}
	}
	keys, err := s.List(ctx, prefix)
	if err != nil {
		return err
	}
	for _, key := range keys {
		if err := s.Delete(ctx, key); err != nil {
			return err
		}
	}
	return nil
}
