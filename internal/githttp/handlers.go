package githttp

import (
	"net/http"

	"github.com/go-git/go-git/v5/plumbing/format/pktline"
	"github.com/go-git/go-git/v5/plumbing/protocol/packp"
	"github.com/go-git/go-git/v5/plumbing/transport"
	"github.com/mikerudolph/artifacts/internal/types"
)

func (s *server) authorize(w http.ResponseWriter, r *http.Request, wantWrite bool) (ns, repo string, ok bool) {
	ns, repo = repoName(r)
	tok := bearerOrBasic(r)
	if tok == "" || s.tokens == nil {
		w.Header().Set("WWW-Authenticate", `Basic realm="git"`)
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return "", "", false
	}
	scope, err := s.tokens.Lookup(r.Context(), ns, repo, tok)
	if err != nil {
		w.Header().Set("WWW-Authenticate", `Basic realm="git"`)
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return "", "", false
	}
	if wantWrite && scope != types.ScopeWrite {
		http.Error(w, "forbidden", http.StatusForbidden)
		return "", "", false
	}
	return ns, repo, true
}

func (s *server) endpoint(ns, repo string) *transport.Endpoint {
	ep, _ := transport.NewEndpoint("http://git/" + ns + "/" + repo)
	return ep
}

func (s *server) infoRefs(w http.ResponseWriter, r *http.Request) {
	svc := r.URL.Query().Get("service")
	wantWrite := svc == transport.ReceivePackServiceName
	ns, repo, ok := s.authorize(w, r, wantWrite)
	if !ok {
		return
	}
	var (
		ar  *packp.AdvRefs
		err error
	)
	switch svc {
	case transport.UploadPackServiceName:
		sess, e := s.git.NewUploadPackSession(s.endpoint(ns, repo), nil)
		if e != nil {
			http.Error(w, e.Error(), http.StatusNotFound)
			return
		}
		ar, err = sess.AdvertisedReferences()
		_ = sess.Close()
	case transport.ReceivePackServiceName:
		sess, e := s.git.NewReceivePackSession(s.endpoint(ns, repo), nil)
		if e != nil {
			http.Error(w, e.Error(), http.StatusNotFound)
			return
		}
		ar, err = sess.AdvertisedReferences()
		_ = sess.Close()
	default:
		http.Error(w, "unsupported service", http.StatusForbidden)
		return
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	ar.Prefix = [][]byte{[]byte("# service=" + svc), pktline.Flush}
	w.Header().Set("Content-Type", "application/x-"+svc+"-advertisement")
	w.Header().Set("Cache-Control", "no-cache")
	_ = ar.Encode(w)
}

func (s *server) uploadPack(w http.ResponseWriter, r *http.Request) {
	ns, repo, ok := s.authorize(w, r, false)
	if !ok {
		return
	}
	sess, err := s.git.NewUploadPackSession(s.endpoint(ns, repo), nil)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	defer func() { _ = sess.Close() }()
	req := packp.NewUploadPackRequest()
	if err := req.Decode(r.Body); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	resp, err := sess.UploadPack(r.Context(), req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/x-git-upload-pack-result")
	w.Header().Set("Cache-Control", "no-cache")
	_ = resp.Encode(w)
}

func (s *server) receivePack(w http.ResponseWriter, r *http.Request) {
	ns, repo, ok := s.authorize(w, r, true)
	if !ok {
		return
	}
	sess, err := s.git.NewReceivePackSession(s.endpoint(ns, repo), nil)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	defer func() { _ = sess.Close() }()
	req := packp.NewReferenceUpdateRequest()
	if err := req.Decode(r.Body); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	status, err := sess.ReceivePack(r.Context(), req)
	if err != nil && status == nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/x-git-receive-pack-result")
	w.Header().Set("Cache-Control", "no-cache")
	if status != nil {
		_ = status.Encode(w)
	}
}
