package repository

import (
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/go-git/go-git/v5/plumbing"
	gitobject "github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/plumbing/storer"
	"github.com/mikerudolph/artifacts/internal/store/meta"
	"github.com/mikerudolph/artifacts/internal/store/object/objecttest"
	"github.com/mikerudolph/artifacts/internal/types"
)

type memoryMeta struct {
	refs        map[types.RepoID]map[string]string
	packs       map[types.RepoID][]types.PackWAL
	checkpoints map[types.RepoID]types.Checkpoint
	forks       map[types.RepoID]types.ForkLineage
}

func newMemoryMeta() *memoryMeta {
	return &memoryMeta{refs: map[types.RepoID]map[string]string{}, packs: map[types.RepoID][]types.PackWAL{},
		checkpoints: map[types.RepoID]types.Checkpoint{}, forks: map[types.RepoID]types.ForkLineage{}}
}

func (*memoryMeta) Accounts() meta.Accounts         { return nil }
func (*memoryMeta) Namespaces() meta.Namespaces     { return nil }
func (*memoryMeta) Repos() meta.Repos               { return nil }
func (m *memoryMeta) Refs() meta.Refs               { return memoryRefs{m} }
func (*memoryMeta) RepoTokens() meta.RepoTokens     { return nil }
func (*memoryMeta) APITokens() meta.APITokens       { return nil }
func (*memoryMeta) Jobs() meta.Jobs                 { return nil }
func (m *memoryMeta) WAL() meta.WAL                 { return memoryWAL{m} }
func (m *memoryMeta) Checkpoints() meta.Checkpoints { return memoryCheckpoints{m} }
func (m *memoryMeta) Forks() meta.Forks             { return memoryForks{m} }
func (m *memoryMeta) RunInTx(_ context.Context, fn func(meta.Store) error) error {
	return fn(m)
}

type memoryRefs struct{ m *memoryMeta }

func (s memoryRefs) Get(_ context.Context, repo types.RepoID, name string) (types.Ref, error) {
	sha, ok := s.m.refs[repo][name]
	if !ok {
		return types.Ref{}, meta.ErrNotFound
	}
	return types.Ref{RepoID: repo, Name: name, SHA: sha}, nil
}
func (s memoryRefs) List(_ context.Context, repo types.RepoID) ([]types.Ref, error) {
	var refs []types.Ref
	for name, sha := range s.m.refs[repo] {
		refs = append(refs, types.Ref{RepoID: repo, Name: name, SHA: sha})
	}
	return refs, nil
}
func (s memoryRefs) CompareAndSwap(_ context.Context, repo types.RepoID, name, old, next string) error {
	if s.m.refs[repo] == nil {
		s.m.refs[repo] = map[string]string{}
	}
	if s.m.refs[repo][name] != old {
		return meta.ErrCASConflict
	}
	if next == "" {
		delete(s.m.refs[repo], name)
	} else {
		s.m.refs[repo][name] = next
	}
	return nil
}
func (s memoryRefs) DeleteAll(_ context.Context, repo types.RepoID) error {
	delete(s.m.refs, repo)
	return nil
}

type memoryWAL struct{ m *memoryMeta }

func (s memoryWAL) Publish(_ context.Context, p types.Publication) (int64, error) {
	sequence := int64(len(s.m.packs[p.Pack.RepoID]))
	if sequence != p.ExpectedSequence {
		return 0, meta.ErrCASConflict
	}
	for _, update := range p.Updates {
		if s.m.refs[p.Pack.RepoID][update.Name] != update.OldSHA {
			return 0, meta.ErrCASConflict
		}
	}
	sequence++
	p.Pack.Sequence = sequence
	p.Pack.CreatedAt = time.Now()
	s.m.packs[p.Pack.RepoID] = append(s.m.packs[p.Pack.RepoID], p.Pack)
	refs := memoryRefs(s)
	for _, update := range p.Updates {
		_ = refs.CompareAndSwap(context.Background(), p.Pack.RepoID, update.Name, update.OldSHA, update.NewSHA)
	}
	return sequence, nil
}
func (s memoryWAL) List(_ context.Context, repo types.RepoID, after, through int64) ([]types.PackWAL, error) {
	var packs []types.PackWAL
	for _, pack := range s.m.packs[repo] {
		if pack.Sequence > after && (through < 0 || pack.Sequence <= through) {
			packs = append(packs, pack)
		}
	}
	return packs, nil
}

type memoryCheckpoints struct{ m *memoryMeta }

func (s memoryCheckpoints) Put(_ context.Context, cp types.Checkpoint) error {
	s.m.checkpoints[cp.RepoID] = cp
	return nil
}
func (s memoryCheckpoints) Get(_ context.Context, repo types.RepoID) (types.Checkpoint, error) {
	cp, ok := s.m.checkpoints[repo]
	if !ok {
		return types.Checkpoint{}, meta.ErrNotFound
	}
	return cp, nil
}

type memoryForks struct{ m *memoryMeta }

func (s memoryForks) CreateSnapshot(context.Context, types.RepoID, types.Repo, bool) (types.Repo, error) {
	return types.Repo{}, errors.New("unused")
}
func (s memoryForks) Get(_ context.Context, repo types.RepoID) (types.ForkLineage, error) {
	line, ok := s.m.forks[repo]
	if !ok {
		return types.ForkLineage{}, meta.ErrNotFound
	}
	return line, nil
}
func (s memoryForks) Children(context.Context, types.RepoID) ([]types.ForkLineage, error) {
	return nil, nil
}

func TestCommitRebuildCompactAndCorruption(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git unavailable")
	}
	ctx := context.Background()
	metadata := newMemoryMeta()
	objects := objecttest.NewMem()
	root := t.TempDir()
	manager, err := New(metadata, objects, root)
	if err != nil {
		t.Fatal(err)
	}
	repo := types.Repo{ID: "repo_1", AccountID: "acct", DefaultBranch: "main", Status: types.RepoReady}
	result, err := manager.Commit(ctx, repo, types.CommitInput{Message: "initial", Files: []types.CommitFile{{Path: "README.md", Content: "hello\n"}}})
	if err != nil || result.Sequence != 1 || result.SHA == "" {
		t.Fatalf("commit %+v %v", result, err)
	}
	repo.WALSequence = 1
	assertReadme(t, manager, repo)
	second, err := manager.Commit(ctx, repo, types.CommitInput{Message: "second", Files: []types.CommitFile{
		{Path: "README.md", Content: "hello\n"}, {Path: "result.txt", Content: "done\n"},
	}})
	if err != nil || second.Sequence != 2 {
		t.Fatalf("second commit %+v %v", second, err)
	}
	repo.WALSequence = 2
	if err := manager.Compact(ctx, repo); err != nil {
		t.Fatal(err)
	}
	if metadata.checkpoints[repo.ID].Sequence != 2 {
		t.Fatal("checkpoint not stored")
	}
	if err := manager.Evict(repo); err != nil {
		t.Fatal(err)
	}
	assertReadme(t, manager, repo)
	pack := filepath.Join(root, "acct", "repo_1", "objects", "pack")
	entries, _ := filepath.Glob(filepath.Join(pack, "*.pack"))
	if len(entries) == 0 || os.WriteFile(entries[0], []byte("corrupt"), 0o600) != nil {
		t.Fatal("could not corrupt pack")
	}
	assertReadme(t, manager, repo)
}

func assertReadme(t *testing.T, manager *Manager, repo types.Repo) {
	t.Helper()
	err := manager.Read(context.Background(), repo, func(store storer.Storer) error {
		ref, err := store.Reference("refs/heads/main")
		if err != nil {
			return err
		}
		commit, err := gitobject.GetCommit(store, ref.Hash())
		if err != nil {
			return err
		}
		file, err := commit.File("README.md")
		if err != nil {
			return err
		}
		text, err := file.Contents()
		if err == nil && text != "hello\n" {
			err = errors.New("unexpected README content")
		}
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestCommitValidationAndConstruction(t *testing.T) {
	repo := types.Repo{DefaultBranch: "main"}
	valid := types.CommitInput{Files: []types.CommitFile{{Path: "a/b", Content: "x"}}}
	if branch, err := validateCommit(repo, valid); err != nil || branch != "main" {
		t.Fatalf("%q %v", branch, err)
	}
	for _, input := range []types.CommitInput{
		{}, {Branch: "bad branch", Files: valid.Files},
		{Files: []types.CommitFile{{Path: "../x"}}},
		{Files: []types.CommitFile{{Path: "x"}, {Path: "x"}}},
		{Files: []types.CommitFile{{Path: ".git/config"}}},
		{Files: []types.CommitFile{{Path: "x", Content: string(make([]byte, maxCommitBytes+1))}}},
	} {
		if _, err := validateCommit(repo, input); err == nil {
			t.Fatalf("accepted %+v", input)
		}
	}
	if _, err := New(nil, objecttest.NewMem(), t.TempDir()); err == nil {
		t.Fatal("accepted nil metadata")
	}
}

func TestImportRPCAndForkLineage(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git unavailable")
	}
	ctx := context.Background()
	sourceDir := t.TempDir()
	if _, err := runGit(ctx, nil, "init", "--initial-branch=main", sourceDir); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sourceDir, "README.md"), []byte("hello\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := runGit(ctx, nil, "-C", sourceDir, "add", "README.md"); err != nil {
		t.Fatal(err)
	}
	if _, err := runGit(ctx, nil, "-C", sourceDir, "-c", "user.name=Agent", "-c", "user.email=a@b",
		"-c", "commit.gpgsign=false", "-c", "core.hooksPath="+t.TempDir(), "commit", "-m", "initial"); err != nil {
		t.Fatal(err)
	}
	metadata := newMemoryMeta()
	manager, err := New(metadata, objecttest.NewMem(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	repo := types.Repo{ID: "repo_import", AccountID: "acct", DefaultBranch: "main", Status: types.RepoImporting}
	if err := manager.Import(ctx, repo, "file://"+sourceDir, "main", 1); err != nil {
		t.Fatal(err)
	}
	repo.Status, repo.WALSequence = types.RepoReady, 1
	assertReadme(t, manager, repo)
	var advertisement strings.Builder
	err = manager.RPC(ctx, repo, "upload-pack", nil, &advertisement, "version=1")
	if err != nil || advertisement.Len() == 0 {
		t.Fatalf("rpc %d %v", advertisement.Len(), err)
	}
	if err := manager.RPC(ctx, repo, "bad", nil, io.Discard, ""); err == nil {
		t.Fatal("accepted bad rpc")
	}
	if err := manager.RPC(ctx, repo, "receive-pack", strings.NewReader("request"), io.Discard, ""); err == nil {
		t.Fatal("accepted receive-pack through read rpc")
	}
	dest := types.Repo{ID: "repo_fork", AccountID: "acct", DefaultBranch: "main", Status: types.RepoReady}
	metadata.forks[dest.ID] = types.ForkLineage{RepoID: dest.ID, ParentRepoID: repo.ID, ParentSequence: 1}
	metadata.refs[dest.ID] = map[string]string{"refs/heads/main": metadata.refs[repo.ID]["refs/heads/main"]}
	assertReadme(t, manager, dest)
	bad := types.Repo{ID: "repo_bad", AccountID: "acct", DefaultBranch: "main", Status: types.RepoImporting}
	if err := manager.Import(ctx, bad, "file:///does/not/exist", "", 0); err == nil {
		t.Fatal("expected import failure")
	}
}

func TestReceiveRejectsInvalidProtocol(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git unavailable")
	}
	metadata := newMemoryMeta()
	manager, err := New(metadata, objecttest.NewMem(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	repo := types.Repo{ID: "repo_receive", AccountID: "acct", DefaultBranch: "main", Status: types.RepoReady}
	if _, err := manager.Receive(context.Background(), repo, strings.NewReader("not pkt-line"), "version=1"); err == nil {
		t.Fatal("accepted invalid receive request")
	}
}

func TestDefaultBranchChangeRefreshesHead(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git unavailable")
	}
	ctx := context.Background()
	metadata := newMemoryMeta()
	manager, err := New(metadata, objecttest.NewMem(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	repo := types.Repo{ID: "branch", AccountID: "acct", DefaultBranch: "main", Status: types.RepoReady}
	result, err := manager.Commit(ctx, repo, types.CommitInput{
		Branch: "develop", Files: []types.CommitFile{{Path: "branch.txt", Content: "develop"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	metadata.refs[repo.ID]["HEAD"] = "ref:refs/heads/develop"
	repo.DefaultBranch, repo.WALSequence = "develop", result.Sequence
	err = manager.Read(ctx, repo, func(store storer.Storer) error {
		head, err := store.Reference(plumbing.HEAD)
		if err != nil || head.Target() != plumbing.NewBranchReferenceName("develop") {
			return errors.New("unexpected symbolic HEAD")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
