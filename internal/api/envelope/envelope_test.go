package envelope

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mikerudolph/artifacts/internal/jobs"
	"github.com/mikerudolph/artifacts/internal/store/meta"
	"github.com/mikerudolph/artifacts/internal/types"
)

func TestOKJSON(t *testing.T) {
	t.Parallel()
	b, err := json.Marshal(OK(map[string]string{"id": "repo_1"}))
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatal(err)
	}
	if got["success"] != true {
		t.Fatalf("%s", b)
	}
	if _, ok := got["result_info"]; ok {
		t.Fatalf("result_info present: %s", b)
	}
	if got["errors"] == nil || got["messages"] == nil {
		t.Fatalf("missing arrays: %s", b)
	}
}

func TestOKInfoAndFail(t *testing.T) {
	t.Parallel()
	env := OKInfo([]int{1}, map[string]int{"count": 1})
	if env.ResultInfo == nil || !env.Success {
		t.Fatalf("%+v", env)
	}
	status, fail := Fail(http.StatusNotFound, APIError{Code: CodeNotFound, Message: "File not found"})
	if status != 404 || fail.Success || fail.Result != nil || fail.Errors[0].Code != CodeNotFound {
		t.Fatalf("%d %+v", status, fail)
	}
	b, err := json.Marshal(fail)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), `"result":null`) {
		t.Fatalf("envelope %s", b)
	}
}

func TestWrite(t *testing.T) {
	t.Parallel()
	rec := httptest.NewRecorder()
	Write(rec, http.StatusCreated, OK("ok"))
	if rec.Code != http.StatusCreated {
		t.Fatalf("code %d", rec.Code)
	}
	if rec.Header().Get("Content-Type") != "application/json" {
		t.Fatal(rec.Header().Get("Content-Type"))
	}
}

func TestFromDomain(t *testing.T) {
	t.Parallel()
	cases := []struct {
		err    error
		status int
		code   int
	}{
		{types.ErrInvalidName, 400, CodeInvalidRepoName},
		{types.ErrInvalidTTL, 400, CodeInvalidTTL},
		{types.ErrInvalidScope, 400, CodeInvalidInput},
		{types.ErrInvalidState, 400, CodeInvalidInput},
		{types.ErrInvalidSort, 400, CodeInvalidInput},
		{types.ErrInvalidStatus, 400, CodeInvalidInput},
		{types.ErrInvalidJurisdiction, 400, CodeInvalidInput},
		{meta.ErrNotFound, 404, CodeNotFound},
		{meta.ErrAlreadyExists, 409, CodeAlreadyExists},
		{jobs.ErrInvalidURL, 400, CodeInvalidURL},
		{jobs.ErrRemoteAuth, 400, CodeRemoteAuthRequired},
		{jobs.ErrUpstream, 502, CodeUpstreamUnavailable},
		{jobs.ErrBusy, 409, CodeImportInProgress},
		{errors.New("boom"), 500, CodeInternalError},
		{nil, 500, CodeInternalError},
	}
	for _, tc := range cases {
		status, apiErr := FromDomain(tc.err)
		if status != tc.status || apiErr.Code != tc.code {
			t.Fatalf("%v -> %d/%d want %d/%d", tc.err, status, apiErr.Code, tc.status, tc.code)
		}
	}
}

func TestAllCodesDefined(t *testing.T) {
	t.Parallel()
	codes := []int{
		CodeInvalidInput, CodeInvalidRepoName, CodeInvalidTTL, CodeInvalidURL,
		CodeRemoteAuthRequired, CodeNotFound, CodeAlreadyExists, CodeImportInProgress,
		CodeForkInProgress, CodeInternalError, CodeUpstreamUnavailable, CodeMemoryLimit,
	}
	seen := map[int]struct{}{}
	for _, c := range codes {
		if _, ok := seen[c]; ok {
			t.Fatalf("duplicate %d", c)
		}
		seen[c] = struct{}{}
	}
}
