package main

import (
	"context"
	"encoding/base64"
	"path/filepath"
	"strings"
	"testing"
)

func TestThreadAndMemoryErrors(t *testing.T) {
	b, _ := testBrain(t)
	ctx := context.Background()
	_, err := b.createThread(ctx, createThreadInput{})
	wantError(t, err)
	_, err = b.createThread(ctx, createThreadInput{Title: "x", Tags: []string{" "}})
	wantError(t, err)
	_, err = b.getThread(ctx, getThreadInput{ThreadID: "bad"})
	wantError(t, err)
	_, err = b.getThread(ctx, getThreadInput{ThreadID: "thread_missing"})
	wantError(t, err)
	for _, input := range []rememberInput{
		{Kind: "note"}, {Content: "x", Kind: "other"},
		{Content: "x", Kind: "note", temporal: temporal{StartsAt: "Tuesday"}},
		{Content: "x", Kind: "note", ThreadIDs: []string{"thread_missing"}},
		{Content: "x", Kind: "note", SourceRunID: "run_missing"},
	} {
		_, err = b.remember(ctx, input)
		wantError(t, err)
	}
	_, err = b.getMemory(ctx, getMemoryInput{MemoryID: "bad"})
	wantError(t, err)
	_, err = b.getMemory(ctx, getMemoryInput{MemoryID: "mem_missing"})
	wantError(t, err)
}

func TestRevisionAndRetractionErrors(t *testing.T) {
	b, _ := testBrain(t)
	ctx := context.Background()
	for _, input := range []reviseMemoryInput{
		{MemoryID: "bad", Content: "x", Reason: "x"},
		{MemoryID: "mem_missing", Reason: "x"},
		{MemoryID: "mem_missing", Content: "x"},
		{MemoryID: "mem_missing", Content: "x", Reason: "x"},
	} {
		_, err := b.reviseMemory(ctx, input)
		wantError(t, err)
	}
	_, err := b.forget(ctx, forgetInput{MemoryID: "bad", Reason: "x"})
	wantError(t, err)
	_, err = b.forget(ctx, forgetInput{MemoryID: "mem_missing"})
	wantError(t, err)
	memory, err := b.remember(ctx, rememberInput{Content: "original", Kind: "fact"})
	if err != nil {
		t.Fatal(err)
	}
	include := false
	got, err := b.getMemory(ctx, getMemoryInput{MemoryID: memory.MemoryID, IncludeHistory: &include})
	if err != nil || got.History != nil {
		t.Fatalf("history suppression: %+v %v", got, err)
	}
	if _, err := b.forget(ctx, forgetInput{MemoryID: memory.MemoryID, Reason: "wrong"}); err != nil {
		t.Fatal(err)
	}
	_, err = b.forget(ctx, forgetInput{MemoryID: memory.MemoryID, Reason: "again"})
	wantError(t, err)
	_, err = b.reviseMemory(ctx, reviseMemoryInput{MemoryID: memory.MemoryID, Content: "new", Reason: "again"})
	wantError(t, err)
}

func TestRunErrors(t *testing.T) {
	b, _ := testBrain(t)
	ctx := context.Background()
	for _, input := range []startRunInput{
		{Purpose: "x"}, {Agent: "x"},
		{Agent: "x", Purpose: "x", ThreadIDs: []string{"thread_missing"}},
		{Agent: "x", Purpose: "x", ParentRunID: "bad"},
		{Agent: "x", Purpose: "x", ParentRunID: "run_missing"},
	} {
		_, err := b.startRun(ctx, input)
		wantError(t, err)
	}
	for _, input := range []finishRunInput{
		{RunID: "bad", Status: "completed", Summary: "x"},
		{RunID: "run_missing", Status: "other", Summary: "x"},
		{RunID: "run_missing", Status: "completed"},
		{RunID: "run_missing", Status: "completed", Summary: "x"},
	} {
		_, err := b.finishRun(ctx, input)
		wantError(t, err)
	}
}

func TestOutputErrorsAndIntegrity(t *testing.T) {
	b, _ := testBrain(t)
	ctx := context.Background()
	run, err := b.startRun(ctx, startRunInput{Agent: "x", Purpose: "produce output"})
	if err != nil {
		t.Fatal(err)
	}
	for _, input := range []storeOutputInput{
		{RunID: "bad", Name: "x", Content: "x"},
		{RunID: run.RunID, Name: "../x", Content: "x"},
		{RunID: run.RunID, Name: "x", Content: "x", Encoding: "hex"},
		{RunID: run.RunID, Name: "x", Content: "%%%", Encoding: "base64"},
		{RunID: "run_missing", Name: "x", Content: "x"},
	} {
		_, err = b.storeOutput(ctx, input)
		wantError(t, err)
	}
	stored, err := b.storeOutput(ctx, storeOutputInput{RunID: run.RunID, Name: "data.txt", Content: "valid"})
	if err != nil {
		t.Fatal(err)
	}
	for _, input := range []readOutputInput{{RunID: "bad", OutputID: stored.OutputID}, {RunID: run.RunID, OutputID: "bad"},
		{RunID: "run_missing", OutputID: stored.OutputID}, {RunID: run.RunID, OutputID: "out_missing"}} {
		_, err = b.readOutput(ctx, input)
		wantError(t, err)
	}
	tamperOutput(t, b, stored.OutputID)
	_, err = b.readOutput(ctx, readOutputInput{RunID: run.RunID, OutputID: stored.OutputID})
	wantError(t, err)
	if _, err := b.finishRun(ctx, finishRunInput{RunID: run.RunID, Status: "failed", Summary: "stopped"}); err != nil {
		t.Fatal(err)
	}
	_, err = b.finishRun(ctx, finishRunInput{RunID: run.RunID, Status: "failed", Summary: "again"})
	wantError(t, err)
	_, err = b.storeOutput(ctx, storeOutputInput{RunID: run.RunID, Name: "late", Content: "x"})
	wantError(t, err)
	if _, _, err := encodeOutput([]byte{0xff}, "utf-8"); err == nil {
		t.Fatal("invalid UTF-8 encoding accepted")
	}
	if _, _, err := encodeOutput([]byte("x"), "other"); err == nil {
		t.Fatal("unknown stored encoding accepted")
	}
	_, _, _ = decodeOutput(base64.StdEncoding.EncodeToString([]byte("x")), "base64")
}

func tamperOutput(t *testing.T, b *brain, outputID string) {
	t.Helper()
	_, err := b.workspace.mutate(context.Background(), "Tamper output "+outputID, func(root string) error {
		state, err := loadState(root)
		if err != nil {
			return err
		}
		doc := state.Outputs[outputID]
		path := filepath.ToSlash(filepath.Dir(doc.Path)) + "/" + doc.Metadata.Name
		return writeRepositoryFile(root, path, []byte("changed"))
	})
	if err != nil {
		t.Fatal(err)
	}
}

func wantError(t *testing.T, err error) {
	t.Helper()
	if err == nil {
		t.Fatal("expected error")
	}
	if strings.Contains(err.Error(), "parallel memory") {
		t.Fatal("stored content leaked in error")
	}
}
