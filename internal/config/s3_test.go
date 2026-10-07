package config

import (
	"strings"
	"testing"
)

func TestS3EncryptionConfiguration(t *testing.T) {
	t.Setenv("ARTIFACTS_STORAGE", "s3")
	t.Setenv("S3_BUCKET", "artifacts")
	for _, tc := range []struct {
		mode, key string
		valid     bool
	}{
		{"", "", true}, {"AES256", "", true}, {"aws:kms", "", true},
		{"aws:kms", "alias/artifacts", true}, {"aws:kms", "arn:aws:kms:us-east-1:123456789012:key/example", true},
		{"", "private-key-id", false}, {"AES256", "private-key-id", false},
		{"aws:kms:dsse", "", false}, {"aes256", "", false}, {"invalid", "", false},
	} {
		t.Setenv("S3_SSE", tc.mode)
		t.Setenv("S3_SSE_KMS_KEY_ID", tc.key)
		cfg, err := Load()
		if (err == nil) != tc.valid {
			t.Fatalf("encryption %q: %v", tc.mode, err)
		}
		if err != nil && strings.Contains(err.Error(), "private-key-id") {
			t.Fatal("configuration error disclosed key identifier")
		}
		if err == nil && (cfg.Storage.S3.SSE != tc.mode || cfg.Storage.S3.SSEKMSKeyID != tc.key) {
			t.Fatal("encryption settings lost")
		}
	}
}
