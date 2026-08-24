package service

import (
	"strings"

	"github.com/mikerudolph/artifacts/internal/types"
)

// RemoteURL is {publicURL}/git/{ns}/{repo}.git.
func RemoteURL(publicURL string, ns types.NamespaceName, repo types.RepoName) string {
	base := strings.TrimRight(publicURL, "/")
	return base + "/git/" + string(ns) + "/" + string(repo) + ".git"
}

func (s *Services) remote(ns types.NamespaceName, repo types.RepoName) string {
	return RemoteURL(s.publicURL, ns, repo)
}

func withRemote(repo types.Repo, publicURL string, ns types.NamespaceName) types.Repo {
	repo.Remote = RemoteURL(publicURL, ns, repo.Name)
	repo.Namespace = ns
	return repo
}

func descPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
