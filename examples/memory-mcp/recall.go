package main

import (
	"context"
	"errors"
	"sort"
	"strings"
	"time"
)

type recallInput struct {
	Query         string   `json:"query,omitempty"`
	Types         []string `json:"types,omitempty"`
	ThreadIDs     []string `json:"thread_ids,omitempty"`
	Kinds         []string `json:"kinds,omitempty"`
	StartsAfter   string   `json:"starts_after,omitempty"`
	StartsBefore  string   `json:"starts_before,omitempty"`
	RecordedAfter string   `json:"recorded_after,omitempty"`
	Limit         int      `json:"limit,omitempty"`
}

type recallMatch struct {
	Type       string   `json:"type"`
	ID         string   `json:"id"`
	Score      int      `json:"score"`
	RecordedAt string   `json:"recorded_at"`
	ThreadIDs  []string `json:"thread_ids,omitempty"`
	Title      string   `json:"title,omitempty"`
	Content    string   `json:"content,omitempty"`
	Kind       string   `json:"kind,omitempty"`
	Status     string   `json:"status,omitempty"`
	Name       string   `json:"name,omitempty"`
	MediaType  string   `json:"media_type,omitempty"`
}

type recallOutput struct {
	Matches []recallMatch `json:"matches"`
}

type recallFilter struct {
	types         map[string]bool
	threads       map[string]bool
	kinds         map[string]bool
	startsAfter   time.Time
	startsBefore  time.Time
	recordedAfter time.Time
	terms         []string
	limit         int
}

func (b *brain) recall(ctx context.Context, input recallInput) (recallOutput, error) {
	filter, err := buildRecallFilter(input)
	if err != nil {
		return recallOutput{}, err
	}
	var result recallOutput
	err = b.workspace.read(ctx, func(root string) error {
		state, err := loadState(root)
		if err != nil {
			return err
		}
		if err := validateRecallThreads(state, filter.threads); err != nil {
			return err
		}
		matches := recallState(root, state, filter)
		if len(matches) > filter.limit {
			matches = matches[:filter.limit]
		}
		result.Matches = matches
		return nil
	})
	return result, err
}

func buildRecallFilter(input recallInput) (recallFilter, error) {
	filter := recallFilter{types: stringSet(input.Types), threads: stringSet(input.ThreadIDs),
		kinds: stringSet(input.Kinds), terms: queryTerms(input.Query), limit: input.Limit}
	for value := range filter.types {
		if err := validateEnum(value, "recall type", recallTypes); err != nil {
			return recallFilter{}, err
		}
	}
	for value := range filter.kinds {
		if err := validateEnum(value, "kind", memoryKinds); err != nil {
			return recallFilter{}, err
		}
	}
	var err error
	if filter.startsAfter, err = optionalTime(input.StartsAfter); err != nil {
		return recallFilter{}, errors.New("starts_after must be RFC3339")
	}
	if filter.startsBefore, err = optionalTime(input.StartsBefore); err != nil {
		return recallFilter{}, errors.New("starts_before must be RFC3339")
	}
	if filter.recordedAfter, err = optionalTime(input.RecordedAfter); err != nil {
		return recallFilter{}, errors.New("recorded_after must be RFC3339")
	}
	if filter.limit == 0 {
		filter.limit = 20
	}
	if filter.limit < 1 || filter.limit > 100 {
		return recallFilter{}, errors.New("limit must be from 1 through 100")
	}
	return filter, nil
}

func validateRecallThreads(state brainState, selected map[string]bool) error {
	for id := range selected {
		if validateID(id, "thread_") != nil {
			return errors.New("invalid thread filter")
		}
		if _, ok := state.Threads[id]; !ok {
			return errors.New("unknown thread filter")
		}
	}
	return nil
}

func recallState(root string, state brainState, filter recallFilter) []recallMatch {
	var matches []recallMatch
	for _, doc := range currentMemories(state, false) {
		matches = appendMatch(matches, doc, memorySearchText(doc), filter)
	}
	for _, doc := range state.Threads {
		matches = appendMatch(matches, doc, threadSearchText(doc), filter)
	}
	for _, doc := range state.Runs {
		matches = appendMatch(matches, doc, runSearchText(doc), filter)
	}
	for _, doc := range state.Outputs {
		matches = appendMatch(matches, doc, outputSearchText(root, doc), filter)
	}
	sort.Slice(matches, func(i, j int) bool {
		if matches[i].Score != matches[j].Score {
			return matches[i].Score > matches[j].Score
		}
		if matches[i].RecordedAt != matches[j].RecordedAt {
			return matches[i].RecordedAt > matches[j].RecordedAt
		}
		return matches[i].ID < matches[j].ID
	})
	return matches
}

func appendMatch(matches []recallMatch, doc document, text string, filter recallFilter) []recallMatch {
	meta := doc.Metadata
	if !matchesFilter(meta, filter) {
		return matches
	}
	score := termScore(text, filter.terms)
	if len(filter.terms) > 0 && score == 0 {
		return matches
	}
	return append(matches, recallMatch{Type: meta.Type, ID: meta.ID, Score: score,
		RecordedAt: meta.RecordedAt, ThreadIDs: cloneStrings(meta.ThreadIDs), Title: meta.Title,
		Content: resultContent(doc), Kind: meta.Kind, Status: meta.Status, Name: meta.Name, MediaType: meta.MediaType})
}

func matchesFilter(meta metadata, filter recallFilter) bool {
	if len(filter.types) > 0 && !filter.types[meta.Type] {
		return false
	}
	threads := meta.ThreadIDs
	if meta.Type == "thread" {
		threads = []string{meta.ID}
	}
	if !hasAll(threads, filter.threads) {
		return false
	}
	if len(filter.kinds) > 0 && (meta.Type != "memory" || !filter.kinds[meta.Kind]) {
		return false
	}
	recorded, err := time.Parse(time.RFC3339Nano, meta.RecordedAt)
	if err != nil || !filter.recordedAfter.IsZero() && !recorded.After(filter.recordedAfter) {
		return false
	}
	return matchesStart(meta.StartsAt, filter.startsAfter, filter.startsBefore)
}

func matchesStart(value string, after, before time.Time) bool {
	if after.IsZero() && before.IsZero() {
		return true
	}
	start, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return false
	}
	return (after.IsZero() || start.After(after)) && (before.IsZero() || start.Before(before))
}

func queryTerms(query string) []string {
	seen := map[string]bool{}
	var terms []string
	for _, term := range strings.Fields(strings.ToLower(query)) {
		if !seen[term] {
			seen[term] = true
			terms = append(terms, term)
		}
	}
	return terms
}

func termScore(text string, terms []string) int {
	text = strings.ToLower(text)
	score := 0
	for _, term := range terms {
		if strings.Contains(text, term) {
			score++
		}
	}
	return score
}

func stringSet(values []string) map[string]bool {
	out := make(map[string]bool, len(values))
	for _, value := range values {
		out[value] = true
	}
	return out
}

func memorySearchText(doc document) string {
	meta := doc.Metadata
	return strings.Join([]string{doc.Content, meta.Kind, strings.Join(meta.ThreadIDs, " "),
		meta.StartsAt, meta.EndsAt, meta.DueAt, meta.Timezone}, " ")
}

func threadSearchText(doc document) string {
	return strings.Join([]string{doc.Metadata.Title, doc.Content, strings.Join(doc.Metadata.Tags, " ")}, " ")
}

func runSearchText(doc document) string {
	return strings.Join([]string{doc.Metadata.Agent, doc.Metadata.Status, doc.Content,
		strings.Join(doc.Metadata.ThreadIDs, " ")}, " ")
}

func resultContent(doc document) string {
	if doc.Metadata.Type == "output" {
		return ""
	}
	return doc.Content
}
