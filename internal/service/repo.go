package service

import (
	"context"

	"github.com/mikerudolph/artifacts/internal/store/meta"
	"github.com/mikerudolph/artifacts/internal/types"
)

// CreateRepo creates a repo, implicitly creating the namespace, and mints a write token.
func (s *Services) CreateRepo(ctx context.Context, account types.AccountID, ns string, in types.CreateRepoInput) (types.CreateRepoResult, error) {
	nsName, repoName, branch, err := parseCreate(ns, in)
	if err != nil {
		return types.CreateRepoResult{}, err
	}
	nspace, err := s.ensureNamespace(ctx, account, nsName)
	if err != nil {
		return types.CreateRepoResult{}, err
	}
	repo, err := s.meta.Repos().Create(ctx, types.Repo{
		NamespaceID:   nspace.ID,
		AccountID:     account,
		Namespace:     nsName,
		Name:          repoName,
		Description:   in.Description,
		DefaultBranch: string(branch),
		ReadOnly:      in.ReadOnly,
		Status:        types.RepoCreating,
		CreatedAt:     s.now(),
	})
	if err != nil {
		return types.CreateRepoResult{}, err
	}
	if err := s.meta.Refs().CompareAndSwap(ctx, repo.ID, "HEAD", "", "ref:refs/heads/"+repo.DefaultBranch); err != nil {
		s.failCreate(ctx, repo, err)
		return types.CreateRepoResult{}, err
	}
	repo, err = s.meta.Repos().Transition(ctx, repo.ID, types.RepoCreating, types.RepoReady, nil)
	if err != nil {
		s.failCreate(ctx, repo, err)
		return types.CreateRepoResult{}, err
	}
	tok, err := s.mintAndStore(ctx, repo.ID, types.ScopeWrite, types.DefaultTTLSeconds)
	if err != nil {
		s.failCreate(ctx, repo, err)
		return types.CreateRepoResult{}, err
	}
	return types.CreateRepoResult{
		ID:            repo.ID,
		Name:          repo.Name,
		Description:   descPtr(repo.Description),
		DefaultBranch: repo.DefaultBranch,
		Remote:        s.remote(account, nsName, repo.Name),
		Token:         tok.Plaintext,
	}, nil
}

func (s *Services) failCreate(ctx context.Context, repo types.Repo, cause error) {
	repo.Status = types.RepoFailed
	repo.Failure = cause.Error()
	_, _ = s.meta.Repos().Update(ctx, repo)
}

func parseCreate(ns string, in types.CreateRepoInput) (types.NamespaceName, types.RepoName, types.BranchName, error) {
	nsName, err := types.ParseNamespaceName(ns)
	if err != nil {
		return "", "", "", err
	}
	repoName, err := types.ParseRepoName(string(in.Name))
	if err != nil {
		return "", "", "", err
	}
	branch := in.DefaultBranch
	if branch == "" {
		branch = types.DefaultBranch
	}
	b, err := types.ParseBranchName(branch)
	if err != nil {
		return "", "", "", err
	}
	return nsName, repoName, b, nil
}

// GetRepo returns a repo with its git remote URL.
func (s *Services) GetRepo(ctx context.Context, account types.AccountID, ns, name string) (types.Repo, error) {
	repo, nsName, err := s.lookupRepo(ctx, account, ns, name)
	if err != nil {
		return types.Repo{}, err
	}
	return withRemote(repo, s.publicURL, account, nsName), nil
}

// ListRepos lists repos in a namespace.
func (s *Services) ListRepos(ctx context.Context, account types.AccountID, ns string, opts meta.ListReposOpts) ([]types.Repo, types.CursorResult, error) {
	nspace, nsName, err := s.lookupNS(ctx, account, ns)
	if err != nil {
		return nil, types.CursorResult{}, err
	}
	opts.NamespaceID = nspace.ID
	repos, info, err := s.meta.Repos().List(ctx, opts)
	if err != nil {
		return nil, types.CursorResult{}, err
	}
	for i := range repos {
		repos[i] = withRemote(repos[i], s.publicURL, account, nsName)
	}
	return repos, info, nil
}

// Lookup resolves a tenant-qualified repository for Git and UI adapters.
func (s *Services) Lookup(ctx context.Context, account types.AccountID, namespace, repo string) (types.Repo, error) {
	return s.GetRepo(ctx, account, namespace, repo)
}

// DeleteRepo marks a repo deleting. Object cleanup is T11.
func (s *Services) DeleteRepo(ctx context.Context, account types.AccountID, ns, name string) (types.RepoID, error) {
	repo, _, err := s.lookupRepo(ctx, account, ns, name)
	if err != nil {
		return "", err
	}
	repo.Status = types.RepoDeleting
	if _, err := s.meta.Repos().Update(ctx, repo); err != nil {
		return "", err
	}
	return repo.ID, nil
}

// UpdateRepo changes mutable settings within the resolved tenant.
func (s *Services) UpdateRepo(ctx context.Context, account types.AccountID, ns, name string, input types.UpdateRepoInput) (types.Repo, error) {
	repo, nsName, err := s.lookupRepo(ctx, account, ns, name)
	if err != nil {
		return types.Repo{}, err
	}
	if input.Description != nil {
		repo.Description = *input.Description
	}
	if input.DefaultBranch != nil {
		branch, err := types.ParseBranchName(*input.DefaultBranch)
		if err != nil {
			return types.Repo{}, err
		}
		repo.DefaultBranch = string(branch)
	}
	if input.ReadOnly != nil {
		repo.ReadOnly = *input.ReadOnly
	}
	var updated types.Repo
	err = s.meta.RunInTx(ctx, func(tx meta.Store) error {
		var updateErr error
		updated, updateErr = tx.Repos().Update(ctx, repo)
		if updateErr != nil || input.DefaultBranch == nil {
			return updateErr
		}
		head, updateErr := tx.Refs().Get(ctx, repo.ID, "HEAD")
		if updateErr != nil {
			return updateErr
		}
		return tx.Refs().CompareAndSwap(ctx, repo.ID, "HEAD", head.SHA, "ref:refs/heads/"+repo.DefaultBranch)
	})
	if err != nil {
		return types.Repo{}, err
	}
	return withRemote(updated, s.publicURL, account, nsName), nil
}

func (s *Services) lookupNS(ctx context.Context, account types.AccountID, ns string) (types.Namespace, types.NamespaceName, error) {
	nsName, err := types.ParseNamespaceName(ns)
	if err != nil {
		return types.Namespace{}, "", err
	}
	nspace, err := s.meta.Namespaces().GetByName(ctx, account, nsName)
	return nspace, nsName, err
}

func (s *Services) lookupRepo(ctx context.Context, account types.AccountID, ns, name string) (types.Repo, types.NamespaceName, error) {
	nspace, nsName, err := s.lookupNS(ctx, account, ns)
	if err != nil {
		return types.Repo{}, "", err
	}
	repoName, err := types.ParseRepoName(name)
	if err != nil {
		return types.Repo{}, "", err
	}
	repo, err := s.meta.Repos().GetByName(ctx, nspace.ID, repoName)
	if err != nil {
		return types.Repo{}, "", err
	}
	if repo.Status == types.RepoDeleted || repo.Status == types.RepoDeleting {
		return types.Repo{}, "", meta.ErrNotFound
	}
	repo.AccountID = account
	repo.Namespace = nsName
	return repo, nsName, nil
}
