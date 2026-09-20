package api

import (
	"testing"

	"github.com/mikerudolph/artifacts/internal/types"
)

func TestCommittedEventsReplayAndCredentialScope(t *testing.T) {
	rebuild := interfaceFixture(t)
	h := rebuild()
	collection := acctBase + "/namespaces/events/repos"
	base := collection + "/source"
	created := keyedRequest(h, "POST", collection, "control", "", `{"name":"source"}`)
	expectStatus(t, created, 200)
	var repo types.CreateRepoResult
	decodeResult(t, created, &repo)
	body := `{"files":[{"path":"a","content":"one"}]}`
	for range 2 {
		expectStatus(t, keyedRequest(h, "POST", base+"/commits", repo.Token, "once", body), 201)
	}
	expectStatus(t, keyedRequest(h, "POST", base+"/commits", repo.Token, "", `{"expected_head":"","files":[{"path":"a","content":"bad"}]}`), 409)
	expectStatus(t, keyedRequest(h, "POST", base+"/commits", repo.Token, "", `{"files":[{"path":"a","content":"two"}]}`), 201)
	h = rebuild()
	pageResponse := keyedRequest(h, "GET", base+"/events?after=0&limit=1", repo.Token, "", "")
	expectStatus(t, pageResponse, 200)
	var page types.EventPage
	decodeResult(t, pageResponse, &page)
	if len(page.Events) != 1 || page.NextAfter != 1 || len(page.Events[0].Updates) != 1 || page.Events[0].RepoID != repo.ID {
		t.Fatalf("page %+v", page)
	}
	if page.Events[0].Updates[0].OldSHA != "" || page.Events[0].Updates[0].NewSHA == "" {
		t.Fatal("missing committed transition")
	}
	next := keyedRequest(h, "GET", base+"/events?after=1", repo.Token, "", "")
	expectStatus(t, next, 200)
	decodeResult(t, next, &page)
	if len(page.Events) != 1 || page.NextAfter != 2 || page.Events[0].Sequence != 2 {
		t.Fatalf("retry or rejected write produced extra event: %+v", page)
	}
	empty := keyedRequest(h, "GET", base+"/events?after=2", repo.Token, "", "")
	expectStatus(t, empty, 200)
	decodeResult(t, empty, &page)
	if len(page.Events) != 0 || page.NextAfter != 2 {
		t.Fatalf("empty page %+v", page)
	}
	for _, query := range []string{"after=-1", "after=no", "limit=0", "limit=101", "limit=no"} {
		expectStatus(t, keyedRequest(h, "GET", base+"/events?"+query, repo.Token, "", ""), 400)
	}
	expectStatus(t, keyedRequest(h, "GET", base+"/events", "wrong", "", ""), 401)
	other := keyedRequest(h, "POST", collection, "control", "", `{"name":"other"}`)
	expectStatus(t, other, 200)
	expectStatus(t, keyedRequest(h, "GET", collection+"/other/events", repo.Token, "", ""), 401)
}
