package config

import "fmt"

func (s S3) Validate() error {
	if s.Bucket == "" {
		return fmt.Errorf("S3_BUCKET is required when ARTIFACTS_STORAGE=s3")
	}
	switch s.SSE {
	case "", "AES256", "aws:kms":
	default:
		return fmt.Errorf("S3_SSE must be unset, AES256, or aws:kms")
	}
	if s.SSEKMSKeyID != "" && s.SSE != "aws:kms" {
		return fmt.Errorf("S3_SSE_KMS_KEY_ID requires S3_SSE=aws:kms")
	}
	return nil
}
