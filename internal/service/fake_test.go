package service

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/mikerudolph/artifacts/internal/store/meta"
	"github.com/mikerudolph/artifacts/internal/types"
)

type fakeStore struct {
	mu         sync.Mutex
	accounts   map[types.AccountID]struct{}
	namespaces map[string]types.Namespace
	repos      map[types.RepoID]types.Repo
	tokens     map[types.TokenID]types.RepoToken
	apiTokens  map[string]types.APIToken
	jobs       map[types.JobID]types.Job
	refs       map[string]string
	seq        int
}

func newFake() *fakeStore {
	return &fakeStore{
		accounts:   map[types.AccountID]struct{}{},
		namespaces: map[string]types.Namespace{},
		repos:      map[types.RepoID]types.Repo{},
		tokens:     map[types.TokenID]types.RepoToken{},
		apiTokens:  map[string]types.APIToken{},
		jobs:       map[types.JobID]types.Job{},
		refs:       map[string]string{},
	}
}

func (f *fakeStore) Accounts() meta.Accounts     { return fakeAccounts{f} }
func (f *fakeStore) Namespaces() meta.Namespaces { return fakeNS{f} }
func (f *fakeStore) Repos() meta.Repos           { return fakeRepos{f} }
func (f *fakeStore) Refs() meta.Refs             { return fakeRefs{f} }
func (f *fakeStore) RepoTokens() meta.RepoTokens { return fakeTokens{f} }
func (f *fakeStore) APITokens() meta.APITokens   { return fakeAPI{f} }
func (f *fakeStore) Jobs() meta.Jobs             { return fakeJobs{f} }
func (f *fakeStore) RunInTx(_ context.Context, fn func(meta.Store) error) error {
	return fn(f)
}

func (f *fakeStore) nextID(prefix string) string {
	f.seq++
	return prefix + itoa(f.seq)
}

type fakeAccounts struct{ *fakeStore }
type fakeNS struct{ *fakeStore }
type fakeRepos struct{ *fakeStore }
type fakeRefs struct{ *fakeStore }
type fakeTokens struct{ *fakeStore }
type fakeAPI struct{ *fakeStore }
type fakeJobs struct{ *fakeStore }

func (a fakeAccounts) Ensure(_ context.Context, id types.AccountID) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.accounts[id] = struct{}{}
	return nil
}

func (a fakeAccounts) Get(_ context.Context, id types.AccountID) (types.AccountID, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if _, ok := a.accounts[id]; !ok {
		return "", meta.ErrNotFound
	}
	return id, nil
}

func nsKey(acct types.AccountID, name types.NamespaceName) string {
	return string(acct) + "/" + string(name)
}

func (n fakeNS) Create(_ context.Context, ns types.Namespace) (types.Namespace, error) {
	n.mu.Lock()
	defer n.mu.Unlock()
	k := nsKey(ns.AccountID, ns.Name)
	if _, ok := n.namespaces[k]; ok {
		return types.Namespace{}, meta.ErrAlreadyExists
	}
	if ns.ID == "" {
		ns.ID = types.NamespaceID(n.nextID("ns_"))
	}
	if ns.CreatedAt.IsZero() {
		ns.CreatedAt = time.Now()
	}
	n.namespaces[k] = ns
	return ns, nil
}

func (n fakeNS) GetByName(_ context.Context, acct types.AccountID, name types.NamespaceName) (types.Namespace, error) {
	n.mu.Lock()
	defer n.mu.Unlock()
	ns, ok := n.namespaces[nsKey(acct, name)]
	if !ok {
		return types.Namespace{}, meta.ErrNotFound
	}
	return ns, nil
}

func (n fakeNS) List(_ context.Context, acct types.AccountID, page types.CursorPage) ([]types.Namespace, types.CursorResult, error) {
	n.mu.Lock()
	defer n.mu.Unlock()
	page = types.NormalizeCursorPage(page)
	var out []types.Namespace
	for _, ns := range n.namespaces {
		if ns.AccountID == acct {
			out = append(out, ns)
		}
	}
	if len(out) > page.Limit {
		out = out[:page.Limit]
	}
	return out, types.CursorResult{PerPage: page.Limit, Count: len(out)}, nil
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [12]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}

func containsFold(s, sub string) bool {
	return sub == "" || strings.Contains(strings.ToLower(s), strings.ToLower(sub))
}
