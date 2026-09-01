package main

import (
	"context"
	"errors"
	"sort"
	"time"
)

type createThreadInput struct {
	Title       string   `json:"title" jsonschema:"title for the persistent context"`
	Description string   `json:"description,omitempty" jsonschema:"description of the context"`
	Tags        []string `json:"tags,omitempty" jsonschema:"searchable context tags"`
}

type createThreadOutput struct {
	ThreadID string `json:"thread_id"`
	Commit   string `json:"commit"`
}

type getThreadInput struct {
	ThreadID         string `json:"thread_id"`
	IncludeRetracted bool   `json:"include_retracted,omitempty"`
}

type threadOutput struct {
	Thread   document   `json:"thread"`
	Memories []document `json:"memories"`
	Runs     []document `json:"runs"`
	Outputs  []metadata `json:"outputs"`
}

type rememberInput struct {
	Content     string   `json:"content"`
	Kind        string   `json:"kind"`
	ThreadIDs   []string `json:"thread_ids,omitempty"`
	SourceRunID string   `json:"source_run_id,omitempty"`
	temporal
}

type rememberOutput struct {
	MemoryID string `json:"memory_id"`
	Commit   string `json:"commit"`
}

type getMemoryInput struct {
	MemoryID       string `json:"memory_id"`
	IncludeHistory *bool  `json:"include_history,omitempty"`
}

type memoryOutput struct {
	Current document   `json:"current"`
	History []document `json:"history,omitempty"`
}

type reviseMemoryInput struct {
	MemoryID  string    `json:"memory_id"`
	Content   string    `json:"content"`
	Reason    string    `json:"reason"`
	Kind      *string   `json:"kind,omitempty"`
	ThreadIDs *[]string `json:"thread_ids,omitempty"`
	temporal
}

type reviseMemoryOutput struct {
	ReplacementMemoryID string `json:"replacement_memory_id"`
	Commit              string `json:"commit"`
}

type forgetInput struct {
	MemoryID string `json:"memory_id"`
	Reason   string `json:"reason"`
}

type forgetOutput struct {
	RetractionMemoryID string `json:"retraction_memory_id"`
	Commit             string `json:"commit"`
}

func (b *brain) createThread(ctx context.Context, input createThreadInput) (createThreadOutput, error) {
	if err := requireText(input.Title, "title"); err != nil {
		return createThreadOutput{}, err
	}
	tags, err := normalizeTags(input.Tags)
	if err != nil {
		return createThreadOutput{}, err
	}
	now := b.now()
	id, err := b.id("thread_", now)
	if err != nil {
		return createThreadOutput{}, err
	}
	doc := document{Metadata: metadata{ID: id, Type: "thread", Title: input.Title,
		Tags: tags, RecordedAt: recordedAt(now)}, Content: input.Description}
	commit, err := b.workspace.mutate(ctx, "Create thread "+id, func(root string) error {
		return writeDocument(root, "threads/"+id+"/thread.md", doc)
	})
	return createThreadOutput{ThreadID: id, Commit: commit}, err
}

func (b *brain) getThread(ctx context.Context, input getThreadInput) (threadOutput, error) {
	if err := validateID(input.ThreadID, "thread_"); err != nil {
		return threadOutput{}, err
	}
	var result threadOutput
	err := b.workspace.read(ctx, func(root string) error {
		state, err := loadState(root)
		if err != nil {
			return err
		}
		thread, ok := state.Threads[input.ThreadID]
		if !ok {
			return errors.New("thread not found")
		}
		result.Thread = thread
		for _, memory := range currentMemories(state, input.IncludeRetracted) {
			if contains(memory.Metadata.ThreadIDs, input.ThreadID) {
				result.Memories = append(result.Memories, memory)
			}
		}
		for _, run := range state.Runs {
			if contains(run.Metadata.ThreadIDs, input.ThreadID) {
				result.Runs = append(result.Runs, run)
				appendRunOutputs(&result, state, run.Metadata.ID)
			}
		}
		sortDocuments(result.Memories)
		sortDocuments(result.Runs)
		return nil
	})
	return result, err
}

func (b *brain) remember(ctx context.Context, input rememberInput) (rememberOutput, error) {
	if err := validateMemoryInput(input.Content, input.Kind, input.temporal); err != nil {
		return rememberOutput{}, err
	}
	now := b.now()
	id, err := b.id("mem_", now)
	if err != nil {
		return rememberOutput{}, err
	}
	input.ThreadIDs = normalizeIDs(input.ThreadIDs)
	doc := document{Metadata: metadata{ID: id, Type: "memory", Kind: input.Kind,
		Operation: "remember", RecordedAt: recordedAt(now), ThreadIDs: input.ThreadIDs,
		SourceRunID: input.SourceRunID, temporal: input.temporal}, Content: input.Content}
	commit, err := b.workspace.mutate(ctx, "Remember "+id, func(root string) error {
		state, err := loadState(root)
		if err != nil {
			return err
		}
		if err := validateRelationships(state, input.ThreadIDs, input.SourceRunID); err != nil {
			return err
		}
		return writeDocument(root, datedPath("memories", id, now), doc)
	})
	return rememberOutput{MemoryID: id, Commit: commit}, err
}

func (b *brain) getMemory(ctx context.Context, input getMemoryInput) (memoryOutput, error) {
	if err := validateID(input.MemoryID, "mem_"); err != nil {
		return memoryOutput{}, err
	}
	var result memoryOutput
	err := b.workspace.read(ctx, func(root string) error {
		state, err := loadState(root)
		if err != nil {
			return err
		}
		current, history, ok := currentMemory(state, input.MemoryID)
		if !ok {
			return errors.New("memory not found")
		}
		result.Current = current
		if input.IncludeHistory == nil || *input.IncludeHistory {
			result.History = history
		}
		return nil
	})
	return result, err
}

func validateMemoryInput(content, kind string, value temporal) error {
	if err := requireText(content, "content"); err != nil {
		return err
	}
	if err := validateEnum(kind, "kind", memoryKinds); err != nil {
		return err
	}
	return validateTemporal(value)
}

func contains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func appendRunOutputs(result *threadOutput, state brainState, runID string) {
	for _, output := range state.Outputs {
		if output.Metadata.RunID == runID {
			result.Outputs = append(result.Outputs, output.Metadata)
		}
	}
}

func sortDocuments(docs []document) {
	sort.Slice(docs, func(i, j int) bool {
		return docs[i].Metadata.RecordedAt > docs[j].Metadata.RecordedAt
	})
}

func (b *brain) reviseMemory(ctx context.Context, input reviseMemoryInput) (reviseMemoryOutput, error) {
	if err := validateID(input.MemoryID, "mem_"); err != nil {
		return reviseMemoryOutput{}, err
	}
	if err := requireText(input.Content, "content"); err != nil {
		return reviseMemoryOutput{}, err
	}
	if err := requireText(input.Reason, "reason"); err != nil {
		return reviseMemoryOutput{}, err
	}
	return b.writeRevision(ctx, input)
}

func (b *brain) writeRevision(ctx context.Context, input reviseMemoryInput) (reviseMemoryOutput, error) {
	now := b.now()
	id, err := b.id("mem_", now)
	if err != nil {
		return reviseMemoryOutput{}, err
	}
	commit, err := b.workspace.mutate(ctx, "Revise memory "+id, func(root string) error {
		state, err := loadState(root)
		if err != nil {
			return err
		}
		current, _, ok := currentMemory(state, input.MemoryID)
		if !ok || current.Metadata.Operation == "retract" {
			return errors.New("memory is not current")
		}
		meta, err := revisionMetadata(state, current, input, id, now)
		if err != nil {
			return err
		}
		return writeDocument(root, datedPath("memories", id, now), document{Metadata: meta, Content: input.Content})
	})
	return reviseMemoryOutput{ReplacementMemoryID: id, Commit: commit}, err
}

func revisionMetadata(state brainState, current document, input reviseMemoryInput, id string, now time.Time) (metadata, error) {
	meta := current.Metadata
	meta.ID, meta.Operation, meta.RecordedAt = id, "revise", recordedAt(now)
	meta.Supersedes, meta.Retracts, meta.Reason = current.Metadata.ID, "", input.Reason
	if input.Kind != nil {
		meta.Kind = *input.Kind
	}
	if input.ThreadIDs != nil {
		meta.ThreadIDs = normalizeIDs(*input.ThreadIDs)
	}
	if input.temporal != (temporal{}) {
		meta.temporal = input.temporal
	}
	if err := validateMemoryInput(input.Content, meta.Kind, meta.temporal); err != nil {
		return metadata{}, err
	}
	if err := validateRelationships(state, meta.ThreadIDs, meta.SourceRunID); err != nil {
		return metadata{}, err
	}
	return meta, nil
}

func (b *brain) forget(ctx context.Context, input forgetInput) (forgetOutput, error) {
	if err := validateID(input.MemoryID, "mem_"); err != nil {
		return forgetOutput{}, err
	}
	if err := requireText(input.Reason, "reason"); err != nil {
		return forgetOutput{}, err
	}
	now := b.now()
	id, err := b.id("mem_", now)
	if err != nil {
		return forgetOutput{}, err
	}
	commit, err := b.workspace.mutate(ctx, "Forget memory "+id, func(root string) error {
		state, err := loadState(root)
		if err != nil {
			return err
		}
		current, _, ok := currentMemory(state, input.MemoryID)
		if !ok || current.Metadata.Operation == "retract" {
			return errors.New("memory is not current")
		}
		meta := current.Metadata
		meta.ID, meta.Operation, meta.RecordedAt = id, "retract", recordedAt(now)
		meta.Supersedes, meta.Retracts, meta.Reason = "", current.Metadata.ID, input.Reason
		return writeDocument(root, datedPath("memories", id, now), document{Metadata: meta, Content: input.Reason})
	})
	return forgetOutput{RetractionMemoryID: id, Commit: commit}, err
}
