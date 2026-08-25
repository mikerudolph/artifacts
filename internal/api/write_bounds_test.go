package api

import (
	"bytes"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDecodeJSONBoundsAndTrailingValues(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		body string
	}{
		{name: "oversized", body: `{"value":"` + strings.Repeat("x", maxJSONBody) + `"}`},
		{name: "trailing value", body: `{"value":"ok"} {"second":true}`},
		{name: "unknown field", body: `{"unknown":true}`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			req := httptest.NewRequest("POST", "/", bytes.NewBufferString(test.body))
			var target struct {
				Value string `json:"value"`
			}
			if err := decodeJSON(httptest.NewRecorder(), req, &target); err == nil {
				t.Fatal("accepted invalid JSON body")
			}
		})
	}
}
