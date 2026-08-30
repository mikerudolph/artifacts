package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEvidenceRedaction(t *testing.T) {
	t.Parallel()
	const control = "control-secret"
	const repository = "art_v1_repository-secret?expires=1234"
	cfg := configuration{
		root:    "https://user:password@example.test/root?token=url-secret&plain=value",
		account: "local", token: control,
	}
	record := newRecorder("abc123", cfg)
	record.addSecret(repository)
	record.gitLog.WriteString("output " + repository + " " + control)
	_ = record.assert("secret safe", true, "detail "+repository)
	record.finish(classified(classProduct, "failure "+control))
	dir := t.TempDir()
	if err := record.write(dir); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"report.json", "report.md", "git.log"} {
		data, err := os.ReadFile(filepath.Join(dir, name)) //nolint:gosec // test-owned temporary directory
		if err != nil {
			t.Fatal(err)
		}
		for _, secret := range []string{control, repository, "art_v1_repository-secret", "url-secret", "password"} {
			if strings.Contains(string(data), secret) {
				t.Fatalf("%s contains %q", name, secret)
			}
		}
	}
	data, err := os.ReadFile(filepath.Join(dir, "report.json")) //nolint:gosec // test-owned temporary directory
	if err != nil {
		t.Fatal(err)
	}
	var got report
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	if got.Configuration.APIToken != "<redacted>" || got.Classification != classProduct {
		t.Fatalf("report config/classification: %+v", got)
	}
	if len(got.Artifacts) != 1 || got.Artifacts[0].Path != "git.log" || len(got.Artifacts[0].SHA256) != 64 {
		t.Fatalf("artifact digest: %+v", got.Artifacts)
	}
}

func TestRedactURL(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		in   string
		deny []string
	}{
		{"userinfo", "https://user:pass@example.test/path", []string{"user", "pass"}},
		{"sensitive query", "https://example.test?access_token=abc&api_key=def&plain=yes", []string{"abc", "def"}},
		{"invalid", "%", []string{"%"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := redactURL(tc.in)
			for _, denied := range tc.deny {
				if strings.Contains(got, denied) {
					t.Fatalf("redactURL()=%q contains %q", got, denied)
				}
			}
		})
	}
}
