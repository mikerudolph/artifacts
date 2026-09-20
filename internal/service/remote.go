package service

import (
	"strings"

	"github.com/mikerudolph/artifacts/internal/types"
)

func RemoteURL(publicURL string, ns types.NamespaceName, repo types.RepoName) string {
	base := strings.TrimRight(publicURL, "/")
	return base + "/git/" + string(ns) + "/" + string(repo) + ".git"
}

func TenantRemoteURL(publicURL string, account types.AccountID, ns types.NamespaceName, repo types.RepoName) string {
	base := strings.TrimRight(publicURL, "/")
	return base + "/git/" + string(account) + "/" + string(ns) + "/" + string(repo) + ".git"
}

func (s *Services) remote(account types.AccountID, ns types.NamespaceName, repo types.RepoName) string {
	return TenantRemoteURL(s.publicURL, account, ns, repo)
}

func withRemote(repo types.Repo, publicURL string, account types.AccountID, ns types.NamespaceName) types.Repo {
	repo.Remote = TenantRemoteURL(publicURL, account, ns, repo.Name)
	repo.AccountID = account
	repo.Namespace = ns
	return repo
}

func descPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
