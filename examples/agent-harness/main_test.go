package main

import (
	"bytes"
	"context"
	"testing"
)

func TestRunUsageAndConfiguration(t *testing.T) {
	tests := []struct {
		name string
		args []string
		url  string
	}{
		{"missing command", nil, "http://localhost"},
		{"unknown command", []string{"wat"}, "http://localhost"},
		{"doctor arguments", []string{"doctor", "extra"}, "http://localhost"},
		{"missing evidence", []string{"verify-core"}, "http://localhost"},
		{"bad flag", []string{"verify-core", "--bad"}, "http://localhost"},
		{"invalid URL", []string{"doctor"}, "%"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("ARTIFACTS_URL", tc.url)
			out, errOut := &bytes.Buffer{}, &bytes.Buffer{}
			if code := run(context.Background(), tc.args, out, errOut); code != 2 {
				t.Fatalf("code=%d stdout=%s stderr=%s", code, out, errOut)
			}
		})
	}
}
