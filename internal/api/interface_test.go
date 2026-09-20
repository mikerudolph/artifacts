package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-git/go-git/v5/plumbing/storer"
	"github.com/mikerudolph/artifacts/internal/auth"
	"github.com/mikerudolph/artifacts/internal/config"
	"github.com/mikerudolph/artifacts/internal/repository"
	"github.com/mikerudolph/artifacts/internal/service"
	"github.com/mikerudolph/artifacts/internal/store/meta/postgres"
	"github.com/mikerudolph/artifacts/internal/store/object/objecttest"
	"github.com/mikerudolph/artifacts/internal/testkit"
	"github.com/mikerudolph/artifacts/internal/types"
)

func interfaceFixture(t *testing.T) func() http.Handler {
	t.Helper()
	dsn := testkit.Postgres(t)
	if err := postgres.Migrate(dsn); err != nil {
		t.Fatal(err)
	}
	objects := objecttest.NewMem()
	return func() http.Handler {
		st, err := postgres.Open(context.Background(), dsn)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { st.(interface{ Close() }).Close() })
		svc := service.New(st, nil, "http://example.test")
		manager, err := repository.New(st, objects, t.TempDir())
		if err != nil {
			t.Fatal(err)
		}
		return NewWithDependencies(svc, config.Config{Auth: config.Auth{Mode: "token", APIToken: "control"}}, Dependencies{
			Repository: manager, RepoAuthorizer: auth.NewRepoAuthorizer(st.RepoTokens(), time.Now),
			ReadGit: func(ctx context.Context, account, ns, name string, visit func(storer.Storer) error) error {
				repo, err := svc.GetRepo(ctx, types.AccountID(account), ns, name)
				if err != nil {
					return err
				}
				return manager.ReadContent(ctx, repo, visit)
			},
		})
	}
}

func keyedRequest(h http.Handler, method, path, token, key, body string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, path, strings.NewReader(body))
	r.Header.Set("Authorization", "Bearer "+token)
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("Idempotency-Key", key)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

func expectStatus(t *testing.T, w *httptest.ResponseRecorder, want int) {
	t.Helper()
	if w.Code != want {
		t.Fatalf("status %d, want %d: %s", w.Code, want, w.Body.String())
	}
}

func TestInterfaceIncrementalWritesAndDurableRetry(t *testing.T) {
	rebuild := interfaceFixture(t)
	h := rebuild()
	collection := acctBase + "/namespaces/work/repos"
	base := collection + "/run"
	create := `{"name":"run","issue_credential":false}`
	first := keyedRequest(h, "POST", collection, "control", "create-run", create)
	expectStatus(t, first, 200)
	var created types.CreateRepoResult
	decodeResult(t, first, &created)
	if created.Credential != nil || created.Token != "" {
		t.Fatal("unexpected credential")
	}
	h = rebuild()
	replay := keyedRequest(h, "POST", collection, "control", "create-run", create)
	if !bytes.Equal(first.Body.Bytes(), replay.Body.Bytes()) {
		t.Fatal("create replay changed")
	}
	expectStatus(t, keyedRequest(h, "POST", collection, "control", "create-run", `{"name":"other","issue_credential":false}`), 409)
	expectStatus(t, keyedRequest(h, "POST", collection, "control", "secret", `{"name":"secret"}`), 400)
	seed := `{"expected_head":"","files":[{"path":"brief.md","content":"input"},{"path":"report.md","content":"draft"}]}`
	w := keyedRequest(h, "POST", base+"/commits", "control", "seed", seed)
	expectStatus(t, w, 201)
	var initial types.CommitResult
	decodeResult(t, w, &initial)
	update := `{"expected_head":"` + initial.SHA + `","files":[{"path":"report.md","content":"done"}]}`
	w = keyedRequest(h, "POST", base+"/commits", "control", "publish", update)
	expectStatus(t, w, 201)
	var published types.CommitResult
	decodeResult(t, w, &published)
	read := doJSON(t, h, "GET", base+"/file?path=brief.md", "control", nil)
	expectStatus(t, read, 200)
	if read.Body.String() != "input" {
		t.Fatal("untouched input changed")
	}
	conflict := keyedRequest(h, "POST", base+"/commits", "control", "", update)
	expectStatus(t, conflict, 409)
	if !strings.Contains(conflict.Body.String(), `"current_head":"`+published.SHA+`"`) {
		t.Fatal("conflict omitted current head")
	}

	expectStatus(t, keyedRequest(h, "POST", base+"/commits", "control", "", `{"files":[{"path":"later","content":"x"}]}`), 201)
	h = rebuild()
	replayed := keyedRequest(h, "POST", base+"/commits", "control", "publish", update)
	expectStatus(t, replayed, 201)
	if !bytes.Equal(w.Body.Bytes(), replayed.Body.Bytes()) {
		t.Fatal("publication replay changed")
	}
	expectStatus(t, keyedRequest(h, "POST", base+"/commits", "control", "publish", seed), 409)
	var packs []types.PackWAL
	decodeResult(t, doJSON(t, h, "GET", base+"/wal", "control", nil), &packs)
	if len(packs) != 3 {
		t.Fatalf("retry published another pack: %d", len(packs))
	}
	assertBranchEdits(t, h, base)
}

func assertBranchEdits(t *testing.T, h http.Handler, base string) {
	t.Helper()

	expectStatus(t, keyedRequest(h, "POST", base+"/commits", "control", "", `{"branch":"work","expected_head":"","deletes":["report.md","later"]}`), 201)
	expectStatus(t, doJSON(t, h, "GET", base+"/file?ref=work&path=brief.md", "control", nil), 200)
	expectStatus(t, keyedRequest(h, "POST", base+"/commits", "control", "", `{"branch":"work","deletes":["brief.md"]}`), 201)
	expectStatus(t, doJSON(t, h, "GET", base+"/file?ref=work&path=brief.md", "control", nil), 404)
	expectStatus(t, keyedRequest(h, "POST", base+"/commits", "control", "", `{"replace":true,"files":[]}`), 201)
	expectStatus(t, doJSON(t, h, "GET", base+"/file?path=brief.md", "control", nil), 404)
}

func TestInterfaceRepositoryCredentialPermissions(t *testing.T) {
	h := interfaceFixture(t)()
	collection := acctBase + "/namespaces/work/repos"
	base := collection + "/run"
	w := doJSON(t, h, "POST", collection, "control", map[string]string{"name": "run"})
	expectStatus(t, w, 200)
	var created types.CreateRepoResult
	decodeResult(t, w, &created)
	if created.Credential == nil || created.Credential.ID == "" || created.Credential.Plaintext != created.Token {
		t.Fatal("initial credential metadata missing")
	}
	expectStatus(t, keyedRequest(h, "POST", base+"/commits", created.Token, "", `{"files":[{"path":"a","content":"x"}]}`), 201)
	credPath := acctBase + "/namespaces/work/credentials"
	w = doJSON(t, h, "POST", credPath, "control", types.CreateTokenInput{Repo: "run", Scope: types.ScopeRead, TTL: 60})
	var read types.CreateTokenResult
	decodeResult(t, w, &read)
	for _, suffix := range []string{"/file?path=a", "/tree", "/log", "/refs", "/raw/main/a"} {
		expectStatus(t, doJSON(t, h, "GET", base+suffix, read.Plaintext, nil), 200)
	}
	expectStatus(t, keyedRequest(h, "POST", base+"/commits", read.Plaintext, "", `{"files":[{"path":"a","content":"x"}]}`), 403)
	for _, suffix := range []string{"", "/credentials", "/jobs", "/wal"} {
		expectStatus(t, doJSON(t, h, "GET", base+suffix, created.Token, nil), 401)
	}
	for _, route := range []struct{ method, path, body string }{
		{"POST", collection, `{"name":"forbidden"}`},
		{"POST", credPath, `{"repo":"run"}`},
		{"DELETE", base, ""},
		{"PATCH", base + "/settings", `{"description":"forbidden"}`},
		{"POST", base + "/fork", `{"name":"forbidden"}`},
	} {
		expectStatus(t, keyedRequest(h, route.method, route.path, created.Token, "", route.body), 401)
	}
	expectStatus(t, doJSON(t, h, "POST", collection, "control", map[string]string{"name": "other"}), 200)
	expectStatus(t, doJSON(t, h, "GET", collection+"/other/refs", created.Token, nil), 401)
	expectStatus(t, doJSON(t, h, "GET", strings.Replace(base, "accounts/local", "accounts/other", 1)+"/refs", created.Token, nil), 401)
	expectStatus(t, doJSON(t, h, "DELETE", credPath+"/"+string(read.ID), "control", nil), 200)
	expectStatus(t, doJSON(t, h, "GET", base+"/file?path=a", read.Plaintext, nil), 401)
	expectStatus(t, doJSON(t, h, "PATCH", base+"/settings", "control", map[string]bool{"read_only": true}), 200)
	expectStatus(t, keyedRequest(h, "POST", base+"/commits", created.Token, "", `{"files":[{"path":"a","content":"x"}]}`), 403)
	expectStatus(t, doJSON(t, h, "GET", base+"/file?path=a", created.Token, nil), 200)
}

func TestInterfaceConcurrentRetryAndValidation(t *testing.T) {
	rebuild := interfaceFixture(t)
	h := rebuild()
	handlers := []http.Handler{h, rebuild()}
	collection := acctBase + "/namespaces/work/repos"
	expectStatus(t, doJSON(t, h, "POST", collection, "control", map[string]string{"name": "run"}), 200)
	base := collection + "/run/commits"
	body := `{"expected_head":"","files":[{"path":"a","content":"x"}]}`
	results := make([]*httptest.ResponseRecorder, 4)
	var wg sync.WaitGroup
	for i := range results {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			results[i] = keyedRequest(handlers[i%len(handlers)], "POST", base, "control", "same", body)
		}(i)
	}
	wg.Wait()
	for _, w := range results {
		expectStatus(t, w, 201)
		if !bytes.Equal(w.Body.Bytes(), results[0].Body.Bytes()) {
			t.Fatal("concurrent retry changed result")
		}
	}
	var commit types.CommitResult
	decodeResult(t, results[0], &commit)
	for i := range results {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			results[i] = keyedRequest(handlers[i%len(handlers)], "POST", base, "control", "", `{"expected_head":"`+commit.SHA+`","files":[{"path":"b","content":"y"}]}`)
		}(i)
	}
	wg.Wait()
	successes := 0
	for _, w := range results {
		if w.Code == 201 {
			successes++
		} else {
			expectStatus(t, w, 409)
		}
	}
	if successes != 1 {
		t.Fatalf("%d writers succeeded at same head", successes)
	}
	for _, input := range []string{
		`{"files":[{"path":"../outside","content":"x"}]}`,
		`{"files":[{"path":"/absolute"}]}`,
		`{"files":[{"path":".git"}]}`,
		`{"files":[{"path":"a"},{"path":"a"}]}`,
		`{"files":[{"path":"a/child"}]}`,
		`{"files":[]} {}`,
		`{"unknown":true}`,
		`{"branch":"bad.lock","files":[{"path":"a"}]}`,
		`{"branch":"a/.hidden","files":[{"path":"a"}]}`,
		`{"expected_head":"nope","files":[{"path":"a"}]}`,
		`{"replace":true,"deletes":["a"]}`,
	} {
		w := keyedRequest(h, "POST", base, "control", "", input)
		expectStatus(t, w, 400)
		if !strings.Contains(w.Body.String(), `"kind":"invalid_input"`) {
			t.Fatal(w.Body.String())
		}
	}
	large, _ := json.Marshal(map[string]any{"files": []types.CommitFile{{Path: "large", Content: strings.Repeat("x", (1<<20)+1)}}})
	expectStatus(t, keyedRequest(h, "POST", base, "control", "", string(large)), 413)
	expectStatus(t, keyedRequest(h, "POST", base, "control", "", strings.Repeat(" ", 2<<20)+body), 413)
}
