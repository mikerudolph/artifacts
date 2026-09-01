package main

import (
	"crypto/rand"
	"errors"
	"io"
	"sort"
	"strings"
	"time"
)

type brain struct {
	workspace *workspace
	now       func() time.Time
	random    io.Reader
}

func newBrain(workspace *workspace) *brain {
	return &brain{workspace: workspace, now: time.Now, random: rand.Reader}
}

func (b *brain) id(prefix string, now time.Time) (string, error) {
	return newID(prefix, now, b.random)
}

func recordedAt(now time.Time) string {
	return now.UTC().Format(time.RFC3339Nano)
}

func validateRelationships(state brainState, threadIDs []string, runID string) error {
	for _, id := range threadIDs {
		if validateID(id, "thread_") != nil {
			return errors.New("invalid thread relationship")
		}
		if _, ok := state.Threads[id]; !ok {
			return errors.New("unknown thread relationship")
		}
	}
	if runID != "" {
		if validateID(runID, "run_") != nil {
			return errors.New("invalid source run")
		}
		if _, ok := state.Runs[runID]; !ok {
			return errors.New("unknown source run")
		}
	}
	return nil
}

func normalizeIDs(values []string) []string {
	seen := make(map[string]bool, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		if !seen[value] {
			seen[value] = true
			out = append(out, value)
		}
	}
	sort.Strings(out)
	return out
}

func requireText(value, label string) error {
	if strings.TrimSpace(value) == "" {
		return errors.New(label + " must be non-empty")
	}
	return nil
}
