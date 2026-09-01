package main

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestBrainEndToEndAndRebuild(t *testing.T) {
	b, remote := testBrain(t)
	fixture := buildWorkflow(t, b)
	assertWorkflowViews(t, b, fixture)
	assertMemoryRetraction(t, b, fixture)
	assertRebuiltLocalClone(t, remote, fixture.TextOutputID)
}

type workflowFixture struct {
	ThreadID       string
	OriginalMemory string
	RevisedMemory  string
	RunID          string
	TextOutputID   string
}

func buildWorkflow(t *testing.T, b *brain) workflowFixture {
	t.Helper()
	ctx := context.Background()
	thread, err := b.createThread(ctx, createThreadInput{
		Title: "Raise proposal", Description: "Prepare for the upcoming review", Tags: []string{" Work ", "work", "Review"},
	})
	if err != nil || thread.ThreadID == "" || thread.Commit == "" {
		t.Fatalf("create thread: %+v %v", thread, err)
	}
	meeting, err := b.remember(ctx, rememberInput{Content: "I'm meeting Alex on Tuesday", Kind: "event",
		ThreadIDs: []string{thread.ThreadID}, temporal: temporal{StartsAt: "2026-09-01T14:00:00-03:00", Timezone: "America/Halifax"}})
	if err != nil {
		t.Fatal(err)
	}
	revised, err := b.reviseMemory(ctx, reviseMemoryInput{MemoryID: meeting.MemoryID,
		Content: "I'm meeting Alex Tuesday to discuss the launch", Reason: "Added the agenda"})
	if err != nil {
		t.Fatal(err)
	}
	run, err := b.startRun(ctx, startRunInput{Agent: "proposal-agent", Purpose: "Research compensation benchmarks",
		ThreadIDs: []string{thread.ThreadID}})
	if err != nil {
		t.Fatal(err)
	}
	textOutput, err := b.storeOutput(ctx, storeOutputInput{RunID: run.RunID, Name: "benchmarks.md",
		Content: "Market compensation benchmarks", MediaType: "text/markdown"})
	if err != nil {
		t.Fatal(err)
	}
	binary := []byte{0, 1, 2, 0xff}
	binaryOutput, err := b.storeOutput(ctx, storeOutputInput{RunID: run.RunID, Name: "chart.bin",
		Content: base64.StdEncoding.EncodeToString(binary), Encoding: "base64", MediaType: "application/octet-stream"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := b.finishRun(ctx, finishRunInput{RunID: run.RunID, Status: "completed", Summary: "Draft is ready"}); err != nil {
		t.Fatal(err)
	}
	assertOutput(t, b, run.RunID, textOutput.OutputID, "Market compensation benchmarks", "utf-8")
	assertOutput(t, b, run.RunID, binaryOutput.OutputID, base64.StdEncoding.EncodeToString(binary), "base64")
	return workflowFixture{ThreadID: thread.ThreadID, OriginalMemory: meeting.MemoryID,
		RevisedMemory: revised.ReplacementMemoryID, RunID: run.RunID, TextOutputID: textOutput.OutputID}
}

func assertWorkflowViews(t *testing.T, b *brain, fixture workflowFixture) {
	t.Helper()
	ctx := context.Background()
	gotMemory, err := b.getMemory(ctx, getMemoryInput{MemoryID: fixture.OriginalMemory})
	if err != nil || len(gotMemory.History) != 2 {
		t.Fatalf("get memory: %+v %v", gotMemory, err)
	}
	threadView, err := b.getThread(ctx, getThreadInput{ThreadID: fixture.ThreadID})
	if err != nil || len(threadView.Memories) != 1 || len(threadView.Runs) != 1 || len(threadView.Outputs) != 2 {
		t.Fatalf("thread view: %+v %v", threadView, err)
	}
	matches, err := b.recall(ctx, recallInput{Query: "market compensation", Types: []string{"output"}, ThreadIDs: []string{fixture.ThreadID}})
	if err != nil || len(matches.Matches) != 1 || matches.Matches[0].ID != fixture.TextOutputID || matches.Matches[0].Score != 2 {
		t.Fatalf("recall output: %+v %v", matches, err)
	}
}

func assertMemoryRetraction(t *testing.T, b *brain, fixture workflowFixture) {
	t.Helper()
	ctx := context.Background()
	forgotten, err := b.forget(ctx, forgetInput{MemoryID: fixture.RevisedMemory, Reason: "Meeting cancelled"})
	if err != nil {
		t.Fatal(err)
	}
	history, err := b.getMemory(ctx, getMemoryInput{MemoryID: fixture.OriginalMemory})
	if err != nil || len(history.History) != 3 || history.Current.Metadata.ID != forgotten.RetractionMemoryID {
		t.Fatalf("memory history: %+v %v", history, err)
	}
	current, err := b.recall(ctx, recallInput{Query: "Alex", Types: []string{"memory"}})
	if err != nil || len(current.Matches) != 0 {
		t.Fatalf("retracted recall: %+v %v", current, err)
	}
}

func assertRebuiltLocalClone(t *testing.T, remote, outputID string) {
	t.Helper()
	ctx := context.Background()
	fresh, err := cloneWorkspace(ctx, remote, "")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = fresh.Close() }()
	rebuilt, err := newBrain(fresh).recall(ctx, recallInput{Query: "proposal", Types: []string{"thread", "run"}})
	if err != nil || len(rebuilt.Matches) != 2 {
		t.Fatalf("rebuilt recall: %+v %v", rebuilt, err)
	}
	log := runTestGit(t, fresh.dir, "log", "--format=%s")
	if strings.Contains(log, "Market compensation") || !strings.Contains(log, outputID) {
		t.Fatalf("unsafe or missing commit messages: %q", log)
	}
}

func TestConcurrentMemoriesAreNotLost(t *testing.T) {
	b, _ := testBrain(t)
	b.now = time.Now
	b.random = rand.Reader
	ctx := context.Background()
	var wait sync.WaitGroup
	errorsFound := make(chan error, 8)
	for i := range 8 {
		wait.Add(1)
		go func() {
			defer wait.Done()
			_, err := b.remember(ctx, rememberInput{Content: "parallel memory", Kind: "note"})
			errorsFound <- err
		}()
		_ = i
	}
	wait.Wait()
	close(errorsFound)
	for err := range errorsFound {
		if err != nil {
			t.Fatal(err)
		}
	}
	result, err := b.recall(ctx, recallInput{Query: "parallel", Types: []string{"memory"}, Limit: 20})
	if err != nil || len(result.Matches) != 8 {
		t.Fatalf("concurrent recall count=%d err=%v", len(result.Matches), err)
	}
}

func assertOutput(t *testing.T, b *brain, runID, outputID, content, encoding string) {
	t.Helper()
	result, err := b.readOutput(context.Background(), readOutputInput{RunID: runID, OutputID: outputID})
	if err != nil || result.Content != content || result.Encoding != encoding || result.SHA256 == "" || result.Size == 0 {
		t.Fatalf("read output: %+v %v", result, err)
	}
}
