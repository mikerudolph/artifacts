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
		Status:        types.RepoReady,
		CreatedAt:     s.now(),
	})
	if err != nil {
		return types.CreateRepoResult{}, err
	}
	tok, err := s.mintAndStore(ctx, repo.ID, types.ScopeWrite, types.DefaultTTLSeconds)
	if err != nil {
		return types.CreateRepoResult{}, err
	}
	return types.CreateRepoResult{
		ID:            repo.ID,
		Name:          repo.Name,
		Description:   descPtr(repo.Description),
		DefaultBranch: repo.DefaultBranch,
		Remote:        s.remote(nsName, repo.Name),
		Token:         tok.Plaintext,
	}, nil
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
	return withRemote(repo, s.publicURL, nsName), nil
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
		repos[i] = withRemote(repos[i], s.publicURL, nsName)
	}
	return repos, info, nil
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
	repo.AccountID = account
	repo.Namespace = nsName
	return repo, nsName, nil
}
