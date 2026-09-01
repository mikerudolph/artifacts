package main

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type brainState struct {
	Threads  map[string]document
	Memories map[string]document
	Runs     map[string]document
	Outputs  map[string]document
}

func loadState(root string) (brainState, error) {
	state := brainState{
		Threads: map[string]document{}, Memories: map[string]document{},
		Runs: map[string]document{}, Outputs: map[string]document{},
	}
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return errors.New("inspect memory repository")
		}
		if entry.IsDir() {
			if entry.Name() == ".git" {
				return filepath.SkipDir
			}
			return nil
		}
		if !isCanonicalDocument(root, path) {
			return nil
		}
		info, err := entry.Info()
		if err != nil || !info.Mode().IsRegular() {
			return errors.New("memory record is not a regular file")
		}
		data, err := os.ReadFile(path) //nolint:gosec // regular file inside disposable clone
		if err != nil {
			return errors.New("read memory record")
		}
		doc, err := parseDocument(data)
		if err != nil {
			return fmt.Errorf("invalid memory record %s: %w", filepath.Base(path), err)
		}
		doc.Path, _ = filepath.Rel(root, path)
		doc.Path = filepath.ToSlash(doc.Path)
		return addDocument(&state, doc)
	})
	return state, err
}

func isCanonicalDocument(root, path string) bool {
	relative, err := filepath.Rel(root, path)
	if err != nil {
		return false
	}
	relative = filepath.ToSlash(relative)
	return strings.HasPrefix(relative, "threads/") && strings.HasSuffix(relative, "/thread.md") ||
		strings.HasPrefix(relative, "memories/") && strings.HasSuffix(relative, ".md") ||
		strings.HasPrefix(relative, "runs/") && (strings.HasSuffix(relative, "/run.md") || strings.HasSuffix(relative, "/output.md"))
}

func addDocument(state *brainState, doc document) error {
	var target map[string]document
	switch doc.Metadata.Type {
	case "thread":
		target = state.Threads
	case "memory":
		target = state.Memories
	case "run":
		target = state.Runs
	case "output":
		target = state.Outputs
	default:
		return errors.New("unknown memory record type")
	}
	if _, exists := target[doc.Metadata.ID]; exists {
		return errors.New("duplicate memory record id")
	}
	target[doc.Metadata.ID] = doc
	return nil
}

func writeDocument(root, relative string, doc document) error {
	data, err := renderDocument(doc)
	if err != nil {
		return err
	}
	return writeRepositoryFile(root, relative, data)
}

func datedPath(category, id string, now time.Time) string {
	return fmt.Sprintf("%s/%04d/%02d/%s.md", category, now.Year(), now.Month(), id)
}

func runPath(id string, now time.Time) string {
	return fmt.Sprintf("runs/%04d/%02d/%s/run.md", now.Year(), now.Month(), id)
}

func outputPaths(run document, outputID, name string) (string, string) {
	base := filepath.ToSlash(filepath.Dir(run.Path)) + "/outputs/" + outputID
	return base + "/output.md", base + "/" + name
}

func currentMemory(state brainState, id string) (document, []document, bool) {
	if _, ok := state.Memories[id]; !ok {
		return document{}, nil, false
	}
	root := id
	ancestors := map[string]bool{}
	for {
		doc, ok := state.Memories[root]
		if !ok || ancestors[root] {
			return document{}, nil, false
		}
		ancestors[root] = true
		prior := doc.Metadata.Supersedes
		if prior == "" {
			prior = doc.Metadata.Retracts
		}
		if prior == "" {
			break
		}
		root = prior
	}
	history := []document{state.Memories[root]}
	seen := map[string]bool{root: true}
	for {
		next, found := newestSuccessor(state, history[len(history)-1].Metadata.ID)
		if !found || seen[next.Metadata.ID] {
			break
		}
		seen[next.Metadata.ID] = true
		history = append(history, next)
	}
	return history[len(history)-1], history, true
}

func newestSuccessor(state brainState, id string) (document, bool) {
	var matches []document
	for _, doc := range state.Memories {
		if doc.Metadata.Supersedes == id || doc.Metadata.Retracts == id {
			matches = append(matches, doc)
		}
	}
	if len(matches) == 0 {
		return document{}, false
	}
	sort.Slice(matches, func(i, j int) bool {
		if matches[i].Metadata.RecordedAt == matches[j].Metadata.RecordedAt {
			return matches[i].Metadata.ID < matches[j].Metadata.ID
		}
		return matches[i].Metadata.RecordedAt > matches[j].Metadata.RecordedAt
	})
	return matches[0], true
}

func currentMemories(state brainState, includeRetracted bool) []document {
	roots := make(map[string]bool)
	for id, doc := range state.Memories {
		if doc.Metadata.Supersedes == "" && doc.Metadata.Retracts == "" {
			roots[id] = true
		}
	}
	out := make([]document, 0, len(roots))
	for id := range roots {
		current, _, ok := currentMemory(state, id)
		if !ok {
			continue
		}
		if current.Metadata.Operation != "retract" || includeRetracted {
			out = append(out, current)
		}
	}
	return out
}

func hasAll(values []string, selected map[string]bool) bool {
	if len(selected) == 0 {
		return true
	}
	for _, value := range values {
		if selected[value] {
			return true
		}
	}
	return false
}
