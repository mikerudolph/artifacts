package ui

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"
)

func TestRESTClientResponseErrors(t *testing.T) {
	client := restClient{handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/bad-status":
			w.WriteHeader(http.StatusBadGateway)
			_, _ = w.Write([]byte("not json"))
		case "/failed-envelope":
			writeFixtureError(w, http.StatusOK, "reported failure")
		case "/bad-result":
			_, _ = w.Write([]byte(`{"success":true,"result":"wrong","errors":[]}`))
		default:
			_, _ = w.Write([]byte(strings.Repeat("x", maxRESTResponseBytes+1)))
		}
	})}
	tests := []struct {
		path string
		want string
	}{
		{path: "/bad-status", want: "REST API returned HTTP 502"},
		{path: "/failed-envelope", want: "reported failure"},
		{path: "/bad-result", want: "decode REST result"},
		{path: "/too-large", want: errResponseTooLarge.Error()},
	}
	for _, test := range tests {
		var result map[string]any
		err := client.getJSON(context.Background(), test.path, &result)
		if err == nil || !strings.Contains(err.Error(), test.want) {
			t.Fatalf("%s: %v", test.path, err)
		}
	}
}

func TestResponseBufferLimitAndStatus(t *testing.T) {
	buffer := newResponseBuffer(2)
	if n, err := buffer.Write([]byte("abc")); n != 2 || !errors.Is(err, errResponseTooLarge) {
		t.Fatalf("first write %d %v", n, err)
	}
	if n, err := buffer.Write([]byte("d")); n != 0 || !errors.Is(err, errResponseTooLarge) {
		t.Fatalf("second write %d %v", n, err)
	}
	buffer.WriteHeader(http.StatusCreated)
	if buffer.status != http.StatusOK || !buffer.overflow {
		t.Fatalf("buffer state %+v", buffer)
	}
	response := &responseBuffer{status: http.StatusInternalServerError, body: *stringsBuffer("bad")}
	if got := decodeResponseError(response).Error(); got != "REST API returned HTTP 500" {
		t.Fatalf("response error %q", got)
	}
}

func stringsBuffer(value string) *bytes.Buffer {
	return bytes.NewBufferString(value)
}
