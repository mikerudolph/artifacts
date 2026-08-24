package service

import (
	"context"
	"time"

	"github.com/mikerudolph/artifacts/internal/store/meta"
	"github.com/mikerudolph/artifacts/internal/types"
)

func (r fakeRepos) Create(_ context.Context, repo types.Repo) (types.Repo, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, existing := range r.repos {
		if existing.NamespaceID == repo.NamespaceID && existing.Name == repo.Name {
			return types.Repo{}, meta.ErrAlreadyExists
		}
	}
	if repo.ID == "" {
		repo.ID = types.RepoID(r.nextID("repo_"))
	}
	if repo.CreatedAt.IsZero() {
		repo.CreatedAt = time.Now()
	}
	r.repos[repo.ID] = repo
	return repo, nil
}

func (r fakeRepos) GetByName(_ context.Context, ns types.NamespaceID, name types.RepoName) (types.Repo, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, repo := range r.repos {
		if repo.NamespaceID == ns && repo.Name == name {
			return repo, nil
		}
	}
	return types.Repo{}, meta.ErrNotFound
}

func (r fakeRepos) GetByID(_ context.Context, id types.RepoID) (types.Repo, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	repo, ok := r.repos[id]
	if !ok {
		return types.Repo{}, meta.ErrNotFound
	}
	return repo, nil
}

func (r fakeRepos) List(_ context.Context, opts meta.ListReposOpts) ([]types.Repo, types.CursorResult, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	opts.Page = types.NormalizeCursorPage(opts.Page)
	var out []types.Repo
	for _, repo := range r.repos {
		if repo.NamespaceID == opts.NamespaceID && containsFold(string(repo.Name), opts.Search) {
			out = append(out, repo)
		}
	}
	if len(out) > opts.Page.Limit {
		out = out[:opts.Page.Limit]
	}
	return out, types.CursorResult{PerPage: opts.Page.Limit, Count: len(out)}, nil
}

func (r fakeRepos) Update(_ context.Context, repo types.Repo) (types.Repo, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.repos[repo.ID]; !ok {
		return types.Repo{}, meta.ErrNotFound
	}
	r.repos[repo.ID] = repo
	return repo, nil
}

func (r fakeRepos) Delete(_ context.Context, id types.RepoID) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.repos[id]; !ok {
		return meta.ErrNotFound
	}
	delete(r.repos, id)
	return nil
}

func (t fakeTokens) Create(_ context.Context, tok types.RepoToken) (types.RepoToken, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if tok.ID == "" {
		tok.ID = types.TokenID(t.nextID("tok_"))
	}
	t.tokens[tok.ID] = tok
	return tok, nil
}

func (t fakeTokens) GetByID(_ context.Context, id types.TokenID) (types.RepoToken, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	tok, ok := t.tokens[id]
	if !ok {
		return types.RepoToken{}, meta.ErrNotFound
	}
	return tok, nil
}

func (t fakeTokens) GetByHash(_ context.Context, hash string) (types.RepoToken, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	for _, tok := range t.tokens {
		if tok.Hash == hash {
			return tok, nil
		}
	}
	return types.RepoToken{}, meta.ErrNotFound
}

func (t fakeTokens) List(_ context.Context, repoID types.RepoID, state types.TokenState, page types.OffsetPage) ([]types.RepoToken, types.OffsetResult, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	page = types.NormalizeOffsetPage(page)
	var out []types.RepoToken
	for _, tok := range t.tokens {
		if tok.RepoID == repoID && (types.IsTokenStateAll(state) || tok.State == state) {
			out = append(out, tok)
		}
	}
	return out, types.OffsetResult{Page: page.Page, PerPage: page.PerPage, Count: len(out), TotalCount: len(out), TotalPages: 1}, nil
}

func (t fakeTokens) Revoke(_ context.Context, id types.TokenID) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	tok, ok := t.tokens[id]
	if !ok {
		return meta.ErrNotFound
	}
	tok.State = types.TokenRevoked
	t.tokens[id] = tok
	return nil
}

func (a fakeAPI) Create(_ context.Context, tok types.APIToken) (types.APIToken, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.apiTokens[tok.Hash] = tok
	return tok, nil
}

func (a fakeAPI) GetByHash(_ context.Context, hash string) (types.APIToken, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	tok, ok := a.apiTokens[hash]
	if !ok {
		return types.APIToken{}, meta.ErrNotFound
	}
	return tok, nil
}

func (j fakeJobs) Create(_ context.Context, job types.Job) (types.Job, error) {
	j.mu.Lock()
	defer j.mu.Unlock()
	if job.ID == "" {
		job.ID = types.JobID(j.nextID("job_"))
	}
	j.jobs[job.ID] = job
	return job, nil
}

func (j fakeJobs) Get(_ context.Context, id types.JobID) (types.Job, error) {
	j.mu.Lock()
	defer j.mu.Unlock()
	job, ok := j.jobs[id]
	if !ok {
		return types.Job{}, meta.ErrNotFound
	}
	return job, nil
}

func (j fakeJobs) Update(_ context.Context, job types.Job) (types.Job, error) {
	j.mu.Lock()
	defer j.mu.Unlock()
	j.jobs[job.ID] = job
	return job, nil
}

func (j fakeJobs) ListByRepo(_ context.Context, repoID types.RepoID) ([]types.Job, error) {
	j.mu.Lock()
	defer j.mu.Unlock()
	var out []types.Job
	for _, job := range j.jobs {
		if job.RepoID == repoID {
			out = append(out, job)
		}
	}
	return out, nil
}

func (r fakeRefs) Get(_ context.Context, repo types.RepoID, name string) (types.Ref, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	sha, ok := r.refs[string(repo)+"|"+name]
	if !ok {
		return types.Ref{}, meta.ErrNotFound
	}
	return types.Ref{RepoID: repo, Name: name, SHA: sha}, nil
}

func (r fakeRefs) List(_ context.Context, repo types.RepoID) ([]types.Ref, error) {
	return nil, nil
}

func (r fakeRefs) CompareAndSwap(_ context.Context, repo types.RepoID, name, _, newSHA string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.refs[string(repo)+"|"+name] = newSHA
	return nil
}

func (r fakeRefs) DeleteAll(_ context.Context, _ types.RepoID) error { return nil }
