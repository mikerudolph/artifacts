package jobs

import (
	"context"
	"errors"
	"net"
	"net/url"
	"strings"
	"time"

	"github.com/go-git/go-git/v5/plumbing/transport"
	"github.com/mikerudolph/artifacts/internal/netpolicy"
	"github.com/mikerudolph/artifacts/internal/types"
)

const (
	importMaxBytes   = int64(512 << 20)
	importMaxObjects = 1_000_000
	importTimeout    = 2 * time.Minute
)

type ipResolver interface {
	LookupIP(context.Context, string, string) ([]net.IP, error)
}

// Import clones a public HTTPS (or file) remote into a new repo.
func (r *Runner) Import(ctx context.Context, account types.AccountID, ns string, name types.RepoName, in types.ImportRepoInput) (types.CreateRepoResult, error) {
	target, err := resolveImportURL(ctx, r.resolver, in.URL)
	if err != nil {
		return types.CreateRepoResult{}, err
	}
	nsName, err := types.ParseNamespaceName(ns)
	if err != nil {
		return types.CreateRepoResult{}, err
	}
	repoName, err := types.ParseRepoName(string(name))
	if err != nil {
		return types.CreateRepoResult{}, err
	}
	if err := r.meta.Accounts().Ensure(ctx, account); err != nil {
		return types.CreateRepoResult{}, err
	}
	nspace, err := r.ensureNS(ctx, account, nsName)
	if err != nil {
		return types.CreateRepoResult{}, err
	}
	repo, err := r.meta.Repos().Create(ctx, types.Repo{
		NamespaceID: nspace.ID, AccountID: account, Name: repoName, Source: in.URL,
		DefaultBranch: firstNonEmpty(in.Branch, types.DefaultBranch), ReadOnly: in.ReadOnly,
		Status: types.RepoImporting, CreatedAt: r.now(),
	})
	if err != nil {
		return types.CreateRepoResult{}, err
	}
	job, err := r.startJob(ctx, repo.ID, types.JobImport)
	if err != nil {
		return types.CreateRepoResult{}, err
	}
	branch, err := r.cloneInto(ctx, repo, in, target)
	if err != nil {
		repo.Status = types.RepoFailed
		repo.Failure = err.Error()
		_, _ = r.meta.Repos().Update(ctx, repo)
		return types.CreateRepoResult{}, r.finishJob(ctx, job, err)
	}
	if in.Branch == "" {
		repo.DefaultBranch = branch
	}
	repo.Status = types.RepoReady
	if _, err := r.meta.Repos().Update(ctx, repo); err != nil {
		return types.CreateRepoResult{}, err
	}
	tok, err := r.mint(ctx, repo.ID)
	if err != nil {
		return types.CreateRepoResult{}, r.finishJob(ctx, job, err)
	}
	if err := r.finishJob(ctx, job, nil); err != nil {
		return types.CreateRepoResult{}, err
	}
	return types.CreateRepoResult{
		ID: repo.ID, Name: repo.Name, DefaultBranch: repo.DefaultBranch,
		Remote: r.tenantRemote(account, nsName, repo.Name), Token: tok.Plaintext,
	}, nil
}

func (r *Runner) cloneInto(ctx context.Context, repo types.Repo, in types.ImportRepoInput, target importTarget) (string, error) {
	if r.imports == nil {
		return "", ErrUpstream
	}
	branch, err := r.imports.ImportControlled(ctx, repo, types.ImportSpec{
		URL: in.URL, Branch: in.Branch, PinnedAddress: target.address, Depth: in.Depth,
		MaxBytes: importMaxBytes, MaxObjects: importMaxObjects, Timeout: importTimeout,
	})
	return branch, mapCloneErr(err)
}

type importTarget struct{ address string }

func validateImportURL(ctx context.Context, raw string) error {
	_, err := resolveImportURL(ctx, nil, raw)
	return err
}

func resolveImportURL(ctx context.Context, resolver ipResolver, raw string) (importTarget, error) {
	host, err := importHostname(raw)
	if err != nil {
		return importTarget{}, err
	}
	if ip := net.ParseIP(host); ip != nil {
		if !netpolicy.IsGloballyRoutable(ip) {
			return importTarget{}, ErrInvalidURL
		}
		return importTarget{address: ip.String()}, nil
	}
	if resolver == nil {
		resolver = net.DefaultResolver
	}
	addrs, err := resolver.LookupIP(ctx, "ip", host)
	if err != nil {
		return importTarget{}, ErrUpstream
	}
	return selectImportAddress(addrs)
}

func importHostname(raw string) (string, error) {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil || strings.Contains(raw, " ") {
		return "", ErrInvalidURL
	}
	host := strings.TrimSuffix(strings.ToLower(u.Hostname()), ".")
	if host == "localhost" {
		return "", ErrInvalidURL
	}
	return host, nil
}

func selectImportAddress(addrs []net.IP) (importTarget, error) {
	var selected string
	for _, ip := range addrs {
		if !netpolicy.IsGloballyRoutable(ip) {
			return importTarget{}, ErrInvalidURL
		}
		if selected == "" || (ip.To4() != nil && net.ParseIP(selected).To4() == nil) {
			selected = ip.String()
		}
	}
	if selected == "" {
		return importTarget{}, ErrUpstream
	}
	return importTarget{address: selected}, nil
}

func mapCloneErr(err error) error {
	if err == nil {
		return nil
	}
	msg := strings.ToLower(err.Error())
	if err == transport.ErrAuthenticationRequired || strings.Contains(msg, "authentication failed") ||
		strings.Contains(msg, "could not read username") || strings.Contains(msg, "http 401") || strings.Contains(msg, "http 403") {
		return ErrRemoteAuth
	}
	if errors.Is(err, context.DeadlineExceeded) || strings.Contains(msg, "import limit exceeded") {
		return ErrUpstream
	}
	if strings.Contains(msg, "repository not found") || strings.Contains(msg, "invalid") {
		return ErrInvalidURL
	}
	return ErrUpstream
}

func (r *Runner) ensureNS(ctx context.Context, account types.AccountID, name types.NamespaceName) (types.Namespace, error) {
	ns, err := r.meta.Namespaces().GetByName(ctx, account, name)
	if err == nil {
		return ns, nil
	}
	return r.meta.Namespaces().Create(ctx, types.Namespace{AccountID: account, Name: name, CreatedAt: r.now()})
}

func firstNonEmpty(v, def string) string {
	if v == "" {
		return def
	}
	return v
}
