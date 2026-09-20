package s3

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	awss3 "github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/mikerudolph/artifacts/internal/store/object"
)

func (s *store) GetRange(ctx context.Context, key string, offset, length int64) (io.ReadCloser, error) {
	if err := object.ValidateKey(key); err != nil {
		return nil, err
	}
	if offset < 0 || length <= 0 || offset > int64(^uint64(0)>>1)-length {
		return nil, os.ErrInvalid
	}
	out, err := s.client.GetObject(ctx, &awss3.GetObjectInput{Bucket: &s.bucket, Key: aws.String(s.full(key)), Range: aws.String(fmt.Sprintf("bytes=%d-%d", offset, offset+length-1))})
	if err != nil {
		if strings.Contains(err.Error(), "InvalidRange") || strings.Contains(err.Error(), "status code: 416") {
			return nil, io.EOF
		}
		return nil, mapErr(err)
	}

	if out.ContentRange == nil || !strings.HasPrefix(*out.ContentRange, fmt.Sprintf("bytes %d-", offset)) {
		_ = out.Body.Close()
		return nil, fmt.Errorf("object endpoint did not honor byte range")
	}
	return out.Body, nil
}
