package main

import (
	"context"
	"encoding/base64"
	"testing"
)

func TestRecallFiltersRankingAndBinaryExclusion(t *testing.T) {
	b, _ := testBrain(t)
	ctx := context.Background()
	thread, _ := b.createThread(ctx, createThreadInput{Title: "Tuesday planning", Tags: []string{"calendar"}})
	first, _ := b.remember(ctx, rememberInput{Content: "alpha beta meeting", Kind: "event", ThreadIDs: []string{thread.ThreadID},
		temporal: temporal{StartsAt: "2026-09-01T14:00:00-03:00", Timezone: "America/Halifax"}})
	_, _ = b.remember(ctx, rememberInput{Content: "alpha proposal", Kind: "goal", ThreadIDs: []string{thread.ThreadID},
		temporal: temporal{DueAt: "2026-09-15T17:00:00-03:00"}})
	ranked, err := b.recall(ctx, recallInput{Query: "alpha beta", Types: []string{"memory"}, Limit: 10})
	if err != nil || len(ranked.Matches) != 2 || ranked.Matches[0].ID != first.MemoryID || ranked.Matches[0].Score != 2 {
		t.Fatalf("ranked=%+v error=%v", ranked, err)
	}
	temporal, err := b.recall(ctx, recallInput{Types: []string{"memory"}, StartsAfter: "2026-09-01T12:00:00-03:00",
		StartsBefore: "2026-09-01T16:00:00-03:00", Kinds: []string{"event"}})
	if err != nil || len(temporal.Matches) != 1 || temporal.Matches[0].ID != first.MemoryID {
		t.Fatalf("temporal=%+v error=%v", temporal, err)
	}
	unfiltered, err := b.recall(ctx, recallInput{Limit: 1})
	if err != nil || len(unfiltered.Matches) != 1 {
		t.Fatalf("unfiltered=%+v error=%v", unfiltered, err)
	}
	run, _ := b.startRun(ctx, startRunInput{Agent: "binary", Purpose: "binary search", ThreadIDs: []string{thread.ThreadID}})
	bytes := append([]byte("hidden-body-term"), 0xff)
	_, _ = b.storeOutput(ctx, storeOutputInput{RunID: run.RunID, Name: "opaque.bin",
		Content: base64.StdEncoding.EncodeToString(bytes), Encoding: "base64"})
	hidden, err := b.recall(ctx, recallInput{Query: "hidden-body-term", Types: []string{"output"}})
	if err != nil || len(hidden.Matches) != 0 {
		t.Fatalf("binary body was searched: %+v %v", hidden, err)
	}
}

func TestRecallValidationErrors(t *testing.T) {
	b, _ := testBrain(t)
	ctx := context.Background()
	inputs := []recallInput{
		{Types: []string{"unknown"}}, {Kinds: []string{"unknown"}},
		{StartsAfter: "tomorrow"}, {StartsBefore: "tomorrow"}, {RecordedAfter: "tomorrow"},
		{Limit: -1}, {Limit: 101}, {ThreadIDs: []string{"bad"}}, {ThreadIDs: []string{"thread_missing"}},
	}
	for _, input := range inputs {
		if _, err := b.recall(ctx, input); err == nil {
			t.Errorf("invalid recall accepted: %+v", input)
		}
	}
	result, err := b.recall(ctx, recallInput{Query: "absent"})
	if err != nil || len(result.Matches) != 0 {
		t.Fatalf("empty result=%+v error=%v", result, err)
	}
}
