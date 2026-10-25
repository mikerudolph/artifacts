package config

import (
	"testing"
	"time"
)

func TestCredentialRetention(t *testing.T) {
	for _, tc := range []struct {
		value string
		want  time.Duration
	}{
		{"", 168 * time.Hour}, {"1h", time.Hour}, {"0", 0}, {"0ms", 0},
	} {
		t.Setenv("ARTIFACTS_CREDENTIAL_RETENTION", tc.value)
		cfg, err := Load()
		if err != nil || cfg.CredentialRetention != tc.want {
			t.Fatalf("retention %q: %s %v", tc.value, cfg.CredentialRetention, err)
		}
	}
	for _, value := range []string{"invalid", "-1h", "999999999999999h"} {
		t.Setenv("ARTIFACTS_CREDENTIAL_RETENTION", value)
		if _, err := Load(); err == nil {
			t.Fatalf("accepted retention %q", value)
		}
	}
	if err := (Config{CredentialRetention: -time.Second}).Validate(); err == nil {
		t.Fatal("accepted negative retention")
	}
}
