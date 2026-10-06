package config

import "testing"

func TestUploadConfiguration(t *testing.T) {
	for _, key := range []string{"ARTIFACTS_COMMIT_MAX_BYTES", "ARTIFACTS_COMMIT_CONCURRENCY", "ARTIFACTS_COMMIT_TIMEOUT"} {
		t.Setenv(key, "")
	}
	got, err := loadUploads()
	if err != nil || got != (Uploads{}).Defaults() {
		t.Fatalf("defaults: %+v %v", got, err)
	}
	for _, tc := range []struct{ key, value string }{
		{"ARTIFACTS_COMMIT_MAX_BYTES", "-1"}, {"ARTIFACTS_COMMIT_MAX_BYTES", "536870913"}, {"ARTIFACTS_COMMIT_MAX_BYTES", "oops"},
		{"ARTIFACTS_COMMIT_CONCURRENCY", "0"}, {"ARTIFACTS_COMMIT_CONCURRENCY", "65"}, {"ARTIFACTS_COMMIT_CONCURRENCY", "oops"},
		{"ARTIFACTS_COMMIT_TIMEOUT", "0s"}, {"ARTIFACTS_COMMIT_TIMEOUT", "2h"}, {"ARTIFACTS_COMMIT_TIMEOUT", "oops"},
	} {
		t.Run(tc.key+tc.value, func(t *testing.T) {
			t.Setenv(tc.key, tc.value)
			if _, err := loadUploads(); err == nil {
				t.Fatal("accepted invalid upload setting")
			}
		})
	}
}
