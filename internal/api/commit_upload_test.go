package api

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"strings"
	"sync"
	"testing"

	"github.com/mikerudolph/artifacts/internal/types"
)

func uploadRequest(t *testing.T, path, token, key, manifest string, files ...string) *http.Request {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreatePart(textproto.MIMEHeader{"Content-Disposition": {`form-data; name="manifest"`}, "Content-Type": {"application/json"}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.WriteString(part, manifest); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < len(files); i += 2 {
		part, err := writer.CreateFormFile(files[i], "ignored")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := io.WriteString(part, files[i+1]); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest("POST", path+"/commits", &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Idempotency-Key", key)
	return req
}

func serveUpload(h http.Handler, req *http.Request) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	return w
}

func TestMultipartCommitRetryAcrossInstances(t *testing.T) {
	rebuild := interfaceFixture(t)
	handlers := []http.Handler{rebuild(), rebuild()}
	collection := acctBase + "/namespaces/binary/repos"
	expectStatus(t, doJSON(t, handlers[0], "POST", collection, "control", map[string]string{"name": "mixed"}), 200)
	base := collection + "/mixed"
	binary := string([]byte{0, 255, 128, 13, 10})
	manifest := `{"expected_head":"","files":[{"path":"asset.bin","part":"b"},{"path":"run.sh","part":"s","mode":"100755"}]}`
	requests := []*http.Request{
		uploadRequest(t, base, "control", "binary", manifest, "b", binary, "s", "echo done\n"),
		uploadRequest(t, base, "control", "binary", manifest, "s", "echo done\n", "b", binary),
	}
	results := make([]*httptest.ResponseRecorder, 2)
	var wg sync.WaitGroup
	for i := range requests {
		wg.Go(func() { results[i] = serveUpload(handlers[i], requests[i]) })
	}
	wg.Wait()
	for _, w := range results {
		expectStatus(t, w, 201)
	}
	if results[0].Body.String() != results[1].Body.String() {
		t.Fatal("retry produced different publication")
	}
	var commit types.CommitResult
	decodeResult(t, results[0], &commit)
	w := doJSON(t, handlers[1], "GET", base+"/file?ref="+commit.SHA+"&path=asset.bin", "control", nil)
	expectStatus(t, w, 200)
	if w.Body.String() != binary {
		t.Fatal("binary bytes changed")
	}
	expectStatus(t, keyedRequest(handlers[0], "POST", base+"/commits", "control", "", `{"files":[{"path":"later","content":"x"}]}`), 201)
	replay := serveUpload(rebuild(), uploadRequest(t, base, "control", "binary", manifest, "b", binary, "s", "echo done\n"))
	expectStatus(t, replay, 201)
	if replay.Body.String() != results[0].Body.String() {
		t.Fatal("replay changed after advancement/cache replacement")
	}
	expectStatus(t, serveUpload(handlers[0], uploadRequest(t, base, "control", "binary", manifest, "b", "changed", "s", "echo done\n")), 409)
	expectStatus(t, serveUpload(handlers[0], uploadRequest(t, base, "control", "other", manifest, "b", binary, "s", "echo done\n")), 409)
	assertMultipartModes(t, handlers[0], base)
}

func assertMultipartModes(t *testing.T, h http.Handler, base string) {
	t.Helper()
	var tree treeResult
	decodeResult(t, doJSON(t, h, "GET", base+"/tree", "control", nil), &tree)
	found := false
	for _, entry := range tree.Entries {
		found = found || entry.Name == "run.sh" && entry.Mode == "0100755"
	}
	if !found {
		t.Fatal("executable mode missing")
	}
	expectStatus(t, serveUpload(h, uploadRequest(t, base, "control", "", `{"files":[{"path":"run.sh","part":"x","mode":"100644"}],"deletes":["later"]}`, "x", "echo new\n")), 201)
	decodeResult(t, doJSON(t, h, "GET", base+"/tree", "control", nil), &tree)
	for _, entry := range tree.Entries {
		if entry.Name == "later" || (entry.Name == "run.sh" && entry.Mode != "0100644") {
			t.Fatal("mode or deletion incorrect")
		}
	}
	expectStatus(t, serveUpload(h, uploadRequest(t, base, "control", "", `{"replace":true}`)), 201)
	expectStatus(t, doJSON(t, h, "GET", base+"/file?path=asset.bin", "control", nil), 404)
}

type hookBody struct {
	io.ReadCloser
	hook func()
}

func (b *hookBody) Read(p []byte) (int, error) {
	if b.hook != nil {
		hook := b.hook
		b.hook = nil
		hook()
	}
	return b.ReadCloser.Read(p)
}

func TestMultipartRevocationDuringReceipt(t *testing.T) {
	h := interfaceFixture(t)()
	collection := acctBase + "/namespaces/binary/repos"
	var created types.CreateRepoResult
	decodeResult(t, doJSON(t, h, "POST", collection, "control", map[string]string{"name": "revoke"}), &created)
	base := collection + "/revoke"
	req := uploadRequest(t, base, created.Token, "revoke", `{"files":[{"path":"x","part":"x"}]}`, "x", "content")
	req.Body = &hookBody{ReadCloser: req.Body, hook: func() {
		expectStatus(t, doJSON(t, h, "DELETE", acctBase+"/namespaces/binary/credentials/"+string(created.Credential.ID), "control", nil), 200)
	}}
	expectStatus(t, serveUpload(h, req), 401)
	var wal []types.PackWAL
	decodeResult(t, doJSON(t, h, "GET", base+"/wal", "control", nil), &wal)
	if len(wal) != 0 {
		t.Fatal("revoked upload published")
	}
}

func TestMultipartInvalidTransport(t *testing.T) {
	h := interfaceFixture(t)()
	collection := acctBase + "/namespaces/binary/repos"
	expectStatus(t, doJSON(t, h, "POST", collection, "control", map[string]string{"name": "invalid"}), 200)
	base := collection + "/invalid"
	for _, change := range []func(*http.Request){
		func(r *http.Request) { r.Header.Set("Content-Type", "multipart/form-data") },
		func(r *http.Request) { r.Header.Set("Content-Encoding", "gzip") },
		func(r *http.Request) { r.Header.Set("Idempotency-Key", " bad") },
		func(r *http.Request) { r.Body = io.NopCloser(strings.NewReader("truncated")) },
	} {
		req := uploadRequest(t, base, "control", "", `{"replace":true}`)
		change(req)
		expectStatus(t, serveUpload(h, req), 400)
	}
	req := uploadRequest(t, base, "control", "", `{"replace":true}`)
	ctx, cancel := context.WithCancel(req.Context())
	cancel()
	expectStatus(t, serveUpload(h, req.WithContext(ctx)), 503)
}

func TestLegacyCommitDigest(t *testing.T) {
	input := types.CommitInput{Files: []types.CommitFile{{Path: "a", Content: "b"}}}
	data, err := json.Marshal(input)
	if err != nil {
		t.Fatal(err)
	}
	want := `{"branch":"","message":"","author":{"name":"","email":"","date":"0001-01-01T00:00:00Z"},"files":[{"path":"a","content":"b"}]}`
	if string(data) != want {
		t.Fatalf("legacy request digest serialization changed: %s", data)
	}
}
