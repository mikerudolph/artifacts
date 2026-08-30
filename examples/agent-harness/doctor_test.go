package main

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) { return f(request) }

func TestDoctorResponses(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		code  int
		body  string
		class string
	}{
		{"healthy", http.StatusOK, `{"success":true,"result":[]}`, classNone},
		{"unauthorized", http.StatusUnauthorized, `{"success":false,"errors":[{"message":"unauthorized"}]}`, classAuthentication},
		{"malformed", http.StatusOK, `{"success":true,"result":"wrong"}`, classMalformed},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			h := newHarness(configuration{root: "http://example.test", account: "local", token: "control"})
			h.client.Transport = roundTripFunc(func(*http.Request) (*http.Response, error) {
				return testHTTPResponse(tc.code, tc.body), nil
			})
			_, err := doctor(context.Background(), h)
			if got := classify(err); got != tc.class {
				t.Fatalf("classification %q, want %q (err=%v)", got, tc.class, err)
			}
		})
	}
}

func TestDoctorUnreachable(t *testing.T) {
	t.Parallel()
	h := newHarness(configuration{root: "http://example.test", account: "local"})
	h.client.Transport = roundTripFunc(func(*http.Request) (*http.Response, error) {
		return nil, &net.DNSError{Err: "missing", Name: "example.test"}
	})
	_, err := doctor(context.Background(), h)
	if got := classify(err); got != classUnreachable {
		t.Fatalf("classification %q, want %q (err=%v)", got, classUnreachable, err)
	}
}

func testHTTPResponse(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body)),
	}
}

func TestFailureClassification(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		err  error
		want string
	}{
		{"success", nil, classNone},
		{"marked assertion", classified(classAssertion, "mismatch"), classAssertion},
		{"authentication", &responseError{status: http.StatusForbidden, message: "no"}, classAuthentication},
		{"malformed", &responseError{malformed: true, message: "shape"}, classMalformed},
		{"server", &responseError{status: http.StatusInternalServerError, message: "failed"}, classProduct},
		{"evidence", classified(classEvidence, "failed"), classEvidence},
		{"network", &net.DNSError{Err: "missing", Name: "invalid"}, classUnreachable},
		{"ordinary", errors.New("failed"), classProduct},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := classify(tc.err); got != tc.want {
				t.Fatalf("classify()=%q want %q", got, tc.want)
			}
		})
	}
}
