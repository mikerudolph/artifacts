package postgres

import (
	"context"
	"encoding/base64"
	"errors"
	"testing"
	"time"

	"github.com/mikerudolph/artifacts/internal/store/meta"
	"github.com/mikerudolph/artifacts/internal/testkit"
	"github.com/mikerudolph/artifacts/internal/types"
)

func TestStore(t *testing.T) {
	ctx := context.Background()
	dsn := testkit.Postgres(t)
	if err := Migrate(dsn); err != nil {
		t.Fatal(err)
	}
	if err := Migrate(dsn); err != nil {
		t.Fatal(err)
	}
	st, err := Open(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if c, ok := st.(*store); ok {
			c.Close()
		}
	})
	acct := types.AccountID("local")
	ns, repo := seedGraph(t, st, acct)
	testRefs(t, st, repo.ID)
	testTokens(t, st, acct, repo.ID)
	testJobs(t, st, repo.ID)
	if err := st.RunInTx(ctx, func(tx meta.Store) error {
		return tx.Refs().CompareAndSwap(ctx, repo.ID, "refs/heads/tx", "", "aaaa")
	}); err != nil {
		t.Fatal(err)
	}
	if err := st.Repos().Delete(ctx, repo.ID); err != nil {
		t.Fatal(err)
	}
	if err := st.Repos().Delete(ctx, repo.ID); !meta.IsNotFound(err) {
		t.Fatalf("delete missing: %v", err)
	}
	assertMissing(t, st, ns.ID)
	if err := st.RunInTx(ctx, func(meta.Store) error { return errors.New("boom") }); err == nil {
		t.Fatal("expected tx error")
	}
	_ = ns
}

func assertMissing(t *testing.T, st meta.Store, nsID types.NamespaceID) {
	t.Helper()
	ctx := context.Background()
	if _, err := st.Repos().GetByID(ctx, "repo_missing"); !meta.IsNotFound(err) {
		t.Fatalf("repo: %v", err)
	}
	if _, err := st.Repos().GetByName(ctx, nsID, "nope"); !meta.IsNotFound(err) {
		t.Fatalf("repo name: %v", err)
	}
	if _, err := st.Namespaces().GetByName(ctx, "local", "nope"); !meta.IsNotFound(err) {
		t.Fatalf("ns: %v", err)
	}
	if _, err := st.RepoTokens().GetByID(ctx, "missing"); !meta.IsNotFound(err) {
		t.Fatalf("tok: %v", err)
	}
	if _, err := st.RepoTokens().GetByHash(ctx, "nohash"); !meta.IsNotFound(err) {
		t.Fatalf("tok hash: %v", err)
	}
	if _, err := st.APITokens().GetByHash(ctx, "nohash"); !meta.IsNotFound(err) {
		t.Fatalf("api: %v", err)
	}
	if _, err := st.Jobs().Get(ctx, "missing"); !meta.IsNotFound(err) {
		t.Fatalf("job: %v", err)
	}
	if _, err := st.Refs().Get(ctx, "repo_missing", "refs/heads/main"); !meta.IsNotFound(err) {
		t.Fatalf("ref: %v", err)
	}
	if _, _, err := st.Repos().List(ctx, meta.ListReposOpts{Sort: "nope"}); err == nil {
		t.Fatal("expected invalid sort")
	}
}

func seedGraph(t *testing.T, st meta.Store, acct types.AccountID) (types.Namespace, types.Repo) {
	t.Helper()
	ctx := context.Background()
	if err := st.Accounts().Ensure(ctx, acct); err != nil {
		t.Fatal(err)
	}
	if _, err := st.Accounts().Get(ctx, acct); err != nil {
		t.Fatal(err)
	}
	if _, err := st.Accounts().Get(ctx, "missing"); !meta.IsNotFound(err) {
		t.Fatalf("missing account: %v", err)
	}
	ns := seedNamespaces(t, st, acct)
	return ns, seedRepo(t, st, ns.ID)
}

func seedNamespaces(t *testing.T, st meta.Store, acct types.AccountID) types.Namespace {
	t.Helper()
	ctx := context.Background()
	ns := mustNS(t, st, acct, "default")
	if _, err := st.Namespaces().Create(ctx, types.Namespace{AccountID: acct, Name: "default"}); !meta.IsAlreadyExists(err) {
		t.Fatalf("dup ns: %v", err)
	}
	got, err := st.Namespaces().GetByName(ctx, acct, "default")
	if err != nil || got.ID != ns.ID {
		t.Fatalf("get ns %v %v", got, err)
	}
	if _, err := st.Namespaces().Create(ctx, types.Namespace{AccountID: acct, Name: "other"}); err != nil {
		t.Fatal(err)
	}
	page1, info, err := st.Namespaces().List(ctx, acct, types.CursorPage{Limit: 1})
	if err != nil || len(page1) != 1 || info.Cursor == "" {
		t.Fatalf("list ns %v %+v %v", page1, info, err)
	}
	page2, _, err := st.Namespaces().List(ctx, acct, types.CursorPage{Limit: 1, Cursor: info.Cursor})
	if err != nil || len(page2) != 1 || page2[0].Name == page1[0].Name {
		t.Fatalf("page2 %v %v", page2, err)
	}
	return ns
}

func seedRepo(t *testing.T, st meta.Store, nsID types.NamespaceID) types.Repo {
	t.Helper()
	ctx := context.Background()
	repo := mustRepo(t, st, nsID, "app")
	if _, err := st.Repos().Create(ctx, types.Repo{NamespaceID: nsID, Name: "app"}); !meta.IsAlreadyExists(err) {
		t.Fatalf("dup repo: %v", err)
	}
	if _, err := st.Repos().GetByName(ctx, nsID, "app"); err != nil {
		t.Fatal(err)
	}
	if _, err := st.Repos().GetByID(ctx, repo.ID); err != nil {
		t.Fatal(err)
	}
	repo.Description = "d"
	if _, err := st.Repos().Update(ctx, repo); err != nil {
		t.Fatal(err)
	}
	repos, _, err := st.Repos().List(ctx, meta.ListReposOpts{NamespaceID: nsID, Search: "ap", Sort: types.SortName, Direction: types.SortAsc})
	if err != nil || len(repos) != 1 {
		t.Fatalf("list repos %v %v", repos, err)
	}
	return repo
}

func testRefs(t *testing.T, st meta.Store, repoID types.RepoID) {
	t.Helper()
	ctx := context.Background()
	refs := st.Refs()
	if err := refs.CompareAndSwap(ctx, repoID, "refs/heads/main", "", "abc"); err != nil {
		t.Fatal(err)
	}
	if err := refs.CompareAndSwap(ctx, repoID, "refs/heads/main", "", "def"); !meta.IsCASConflict(err) {
		t.Fatalf("create conflict: %v", err)
	}
	if err := refs.CompareAndSwap(ctx, repoID, "refs/heads/main", "abc", "def"); err != nil {
		t.Fatal(err)
	}
	if err := refs.CompareAndSwap(ctx, repoID, "refs/heads/main", "nope", "zzz"); !meta.IsCASConflict(err) {
		t.Fatalf("update conflict: %v", err)
	}
	got, err := refs.Get(ctx, repoID, "refs/heads/main")
	if err != nil || got.SHA != "def" {
		t.Fatalf("get ref %v %v", got, err)
	}
	all, err := refs.List(ctx, repoID)
	if err != nil || len(all) != 1 {
		t.Fatalf("list refs %v %v", all, err)
	}
	if err := refs.CompareAndSwap(ctx, repoID, "refs/heads/main", "def", ""); err != nil {
		t.Fatal(err)
	}
	if err := refs.CompareAndSwap(ctx, repoID, "refs/heads/main", "def", ""); !meta.IsCASConflict(err) {
		t.Fatalf("delete conflict: %v", err)
	}
	if err := refs.CompareAndSwap(ctx, repoID, "refs/heads/x", "", "1"); err != nil {
		t.Fatal(err)
	}
	if err := refs.DeleteAll(ctx, repoID); err != nil {
		t.Fatal(err)
	}
	if err := refs.CompareAndSwap(ctx, repoID, "n", "", ""); err != nil {
		t.Fatal(err)
	}
}

func testTokens(t *testing.T, st meta.Store, acct types.AccountID, repoID types.RepoID) {
	t.Helper()
	ctx := context.Background()
	tok, err := st.RepoTokens().Create(ctx, types.RepoToken{
		RepoID: repoID, Hash: "h1", Scope: types.ScopeWrite, ExpiresAt: time.Now().Add(time.Hour),
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.RepoTokens().GetByID(ctx, tok.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := st.RepoTokens().GetByHash(ctx, "h1"); err != nil {
		t.Fatal(err)
	}
	list, info, err := st.RepoTokens().List(ctx, repoID, types.TokenActive, types.OffsetPage{})
	if err != nil || len(list) != 1 || info.TotalCount != 1 {
		t.Fatalf("list tok %v %v %v", list, info, err)
	}
	if err := st.RepoTokens().Revoke(ctx, tok.ID); err != nil {
		t.Fatal(err)
	}
	if err := st.RepoTokens().Revoke(ctx, "missing"); !meta.IsNotFound(err) {
		t.Fatalf("revoke missing: %v", err)
	}
	api, err := st.APITokens().Create(ctx, types.APIToken{AccountID: acct, Hash: "apihash"})
	if err != nil || api.ID == "" {
		t.Fatalf("api %v %v", api, err)
	}
	if _, err := st.APITokens().GetByHash(ctx, "apihash"); err != nil {
		t.Fatal(err)
	}
}

func testJobs(t *testing.T, st meta.Store, repoID types.RepoID) {
	t.Helper()
	ctx := context.Background()
	job, err := st.Jobs().Create(ctx, types.Job{RepoID: repoID, Kind: types.JobImport})
	if err != nil {
		t.Fatal(err)
	}
	job.Status = types.JobSucceeded
	if _, err := st.Jobs().Update(ctx, job); err != nil {
		t.Fatal(err)
	}
	if _, err := st.Jobs().Get(ctx, job.ID); err != nil {
		t.Fatal(err)
	}
	all, err := st.Jobs().ListByRepo(ctx, repoID)
	if err != nil || len(all) != 1 {
		t.Fatalf("jobs %v %v", all, err)
	}
}

func TestOpenAndCursorErrors(t *testing.T) {
	t.Parallel()
	if _, err := Open(context.Background(), "postgres://127.0.0.1:1/none?sslmode=disable"); err == nil {
		t.Fatal("expected open error")
	}
	if wrap(nil) != nil {
		t.Fatal("wrap nil")
	}
	if _, _, err := decodeCursor("%%%"); err == nil {
		t.Fatal("expected cursor error")
	}
	if _, _, err := decodeCursor("YQ"); err == nil { // "a"
		t.Fatal("expected cursor error")
	}
	badTime := base64.RawURLEncoding.EncodeToString([]byte("not-a-time|id"))
	if _, _, err := decodeCursor(badTime); err == nil {
		t.Fatal("expected time cursor error")
	}
	if _, ok := repoOrder("nope", types.SortDesc); ok {
		t.Fatal("expected invalid sort")
	}
	cur := encodeCursor(time.Now(), "abc")
	if _, id, err := decodeCursor(cur); err != nil || id != "abc" {
		t.Fatalf("roundtrip %q %v", id, err)
	}
}

func mustNS(t *testing.T, st meta.Store, acct types.AccountID, name types.NamespaceName) types.Namespace {
	t.Helper()
	ns, err := st.Namespaces().Create(context.Background(), types.Namespace{AccountID: acct, Name: name})
	if err != nil {
		t.Fatal(err)
	}
	return ns
}

func mustRepo(t *testing.T, st meta.Store, ns types.NamespaceID, name types.RepoName) types.Repo {
	t.Helper()
	repo, err := st.Repos().Create(context.Background(), types.Repo{NamespaceID: ns, Name: name})
	if err != nil {
		t.Fatal(err)
	}
	return repo
}
