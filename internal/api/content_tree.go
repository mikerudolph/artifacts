package api

import (
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/filemode"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/plumbing/storer"
	"github.com/mikerudolph/artifacts/internal/store/meta"
	"github.com/mikerudolph/artifacts/internal/types"
)

type treeResult struct {
	Ref     string            `json:"ref"`
	Commit  string            `json:"commit"`
	Tree    string            `json:"tree"`
	Path    string            `json:"path"`
	Entries []types.TreeEntry `json:"entries"`
}

func (s *server) handleTreeAt(w http.ResponseWriter, r *http.Request) {
	var result treeResult
	err := s.readRepo(r.Context(), chi.URLParam(r, "account_id"), chi.URLParam(r, "namespace"), chi.URLParam(r, "name"),
		func(st storer.Storer) error {
			var err error
			result, err = readTreeAt(st, r.URL.Query().Get("ref"), r.URL.Query().Get("path"))
			return err
		})
	if err != nil {
		writeErr(w, err)
		return
	}
	writeOK(w, http.StatusOK, result)
}

func readTreeAt(st storer.Storer, ref, path string) (treeResult, error) {
	hash, err := resolveRef(st, ref)
	if err != nil {
		return treeResult{}, err
	}
	commit, err := object.GetCommit(st, hash)
	if err != nil {
		return treeResult{}, meta.ErrNotFound
	}
	tree, err := commit.Tree()
	if err != nil {
		return treeResult{}, meta.ErrNotFound
	}
	path = strings.Trim(path, "/")
	if path != "" {
		entry, findErr := tree.FindEntry(path)
		if findErr != nil || entry.Mode != filemode.Dir {
			return treeResult{}, meta.ErrNotFound
		}
		tree, err = object.GetTree(st, entry.Hash)
		if err != nil {
			return treeResult{}, meta.ErrNotFound
		}
	}
	if ref == "" {
		ref = "HEAD"
	}
	return treeResult{Ref: ref, Commit: hash.String(), Tree: tree.Hash.String(), Path: path, Entries: treeEntries(tree)}, nil
}

func (s *server) handleTree(w http.ResponseWriter, r *http.Request) {
	var out []types.TreeEntry
	err := s.readRepo(r.Context(), chi.URLParam(r, "account_id"), chi.URLParam(r, "namespace"), chi.URLParam(r, "name"),
		func(st storer.Storer) error {
			tr, err := object.GetTree(st, plumbing.NewHash(chi.URLParam(r, "hash")))
			if err != nil {
				return meta.ErrNotFound
			}
			out = treeEntries(tr)
			return nil
		})
	if err != nil {
		writeErr(w, err)
		return
	}
	writeOK(w, http.StatusOK, out)
}

func treeEntries(tree *object.Tree) []types.TreeEntry {
	out := make([]types.TreeEntry, 0, len(tree.Entries))
	for _, entry := range tree.Entries {
		kind := "blob"
		if entry.Mode == filemode.Dir {
			kind = "tree"
		}
		out = append(out, types.TreeEntry{
			Mode: entry.Mode.String(), Type: kind, Hash: entry.Hash.String(), Name: entry.Name,
		})
	}
	return out
}
