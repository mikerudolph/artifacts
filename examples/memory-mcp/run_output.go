package main

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"unicode/utf8"
)

type startRunInput struct {
	Agent       string   `json:"agent"`
	Purpose     string   `json:"purpose"`
	ThreadIDs   []string `json:"thread_ids,omitempty"`
	ParentRunID string   `json:"parent_run_id,omitempty"`
}

type startRunOutput struct {
	RunID  string `json:"run_id"`
	Commit string `json:"commit"`
}

type finishRunInput struct {
	RunID   string `json:"run_id"`
	Status  string `json:"status"`
	Summary string `json:"summary"`
}

type finishRunOutput struct {
	RunID  string `json:"run_id"`
	Commit string `json:"commit"`
}

type storeOutputInput struct {
	RunID     string `json:"run_id"`
	Name      string `json:"name"`
	Content   string `json:"content"`
	Encoding  string `json:"encoding,omitempty"`
	MediaType string `json:"media_type,omitempty"`
}

type storeOutputOutput struct {
	OutputID string `json:"output_id"`
	SHA256   string `json:"sha256"`
	Size     int    `json:"size"`
	Commit   string `json:"commit"`
}

type readOutputInput struct {
	RunID    string `json:"run_id"`
	OutputID string `json:"output_id"`
}

type readOutputOutput struct {
	Name      string `json:"name"`
	Content   string `json:"content"`
	Encoding  string `json:"encoding"`
	MediaType string `json:"media_type"`
	SHA256    string `json:"sha256"`
	Size      int    `json:"size"`
}

func (b *brain) startRun(ctx context.Context, input startRunInput) (startRunOutput, error) {
	if err := requireText(input.Agent, "agent"); err != nil {
		return startRunOutput{}, err
	}
	if err := requireText(input.Purpose, "purpose"); err != nil {
		return startRunOutput{}, err
	}
	input.ThreadIDs = normalizeIDs(input.ThreadIDs)
	now := b.now()
	id, err := b.id("run_", now)
	if err != nil {
		return startRunOutput{}, err
	}
	doc := document{Metadata: metadata{ID: id, Type: "run", Agent: input.Agent,
		Status: "running", ParentRunID: input.ParentRunID, ThreadIDs: input.ThreadIDs,
		RecordedAt: recordedAt(now)}, Content: input.Purpose}
	commit, err := b.workspace.mutate(ctx, "Start run "+id, func(root string) error {
		state, err := loadState(root)
		if err != nil {
			return err
		}
		if err := validateRelationships(state, input.ThreadIDs, ""); err != nil {
			return err
		}
		if input.ParentRunID != "" {
			if validateID(input.ParentRunID, "run_") != nil {
				return errors.New("invalid parent run")
			}
			if _, ok := state.Runs[input.ParentRunID]; !ok {
				return errors.New("unknown parent run")
			}
		}
		return writeDocument(root, runPath(id, now), doc)
	})
	return startRunOutput{RunID: id, Commit: commit}, err
}

func (b *brain) finishRun(ctx context.Context, input finishRunInput) (finishRunOutput, error) {
	if err := validateID(input.RunID, "run_"); err != nil {
		return finishRunOutput{}, err
	}
	statuses := map[string]bool{"completed": true, "failed": true, "cancelled": true}
	if err := validateEnum(input.Status, "run status", statuses); err != nil {
		return finishRunOutput{}, err
	}
	if err := requireText(input.Summary, "summary"); err != nil {
		return finishRunOutput{}, err
	}
	now := b.now()
	commit, err := b.workspace.mutate(ctx, "Finish run "+input.RunID, func(root string) error {
		state, err := loadState(root)
		if err != nil {
			return err
		}
		doc, ok := state.Runs[input.RunID]
		if !ok {
			return errors.New("run not found")
		}
		if doc.Metadata.Status != "running" {
			return errors.New("run is already finished")
		}
		doc.Metadata.Status = input.Status
		doc.Metadata.FinishedAt = recordedAt(now)
		doc.Content += "\n\n## Summary\n\n" + input.Summary
		return writeDocument(root, doc.Path, doc)
	})
	return finishRunOutput{RunID: input.RunID, Commit: commit}, err
}

func (b *brain) storeOutput(ctx context.Context, input storeOutputInput) (storeOutputOutput, error) {
	if err := validateID(input.RunID, "run_"); err != nil {
		return storeOutputOutput{}, err
	}
	if err := validateOutputName(input.Name); err != nil {
		return storeOutputOutput{}, err
	}
	data, encoding, err := decodeOutput(input.Content, input.Encoding)
	if err != nil {
		return storeOutputOutput{}, err
	}
	if input.MediaType == "" {
		input.MediaType = "text/plain"
	}
	now := b.now()
	id, err := b.id("out_", now)
	if err != nil {
		return storeOutputOutput{}, err
	}
	digest := fmt.Sprintf("%x", sha256.Sum256(data))
	result := storeOutputOutput{OutputID: id, SHA256: digest, Size: len(data)}
	result.Commit, err = b.workspace.mutate(ctx, "Store output "+id, func(root string) error {
		state, err := loadState(root)
		if err != nil {
			return err
		}
		run, ok := state.Runs[input.RunID]
		if !ok {
			return errors.New("run not found")
		}
		if run.Metadata.Status != "running" {
			return errors.New("run is already finished")
		}
		metaPath, dataPath := outputPaths(run, id, input.Name)
		meta := metadata{ID: id, Type: "output", RunID: input.RunID, Name: input.Name,
			RecordedAt: recordedAt(now), MediaType: input.MediaType, SHA256: digest,
			Size: len(data), StoredEncode: encoding, ThreadIDs: cloneStrings(run.Metadata.ThreadIDs)}
		if err := writeDocument(root, metaPath, document{Metadata: meta}); err != nil {
			return err
		}
		return writeRepositoryFile(root, dataPath, data)
	})
	return result, err
}

func (b *brain) readOutput(ctx context.Context, input readOutputInput) (readOutputOutput, error) {
	if err := validateID(input.RunID, "run_"); err != nil {
		return readOutputOutput{}, err
	}
	if err := validateID(input.OutputID, "out_"); err != nil {
		return readOutputOutput{}, err
	}
	var result readOutputOutput
	err := b.workspace.read(ctx, func(root string) error {
		state, err := loadState(root)
		if err != nil {
			return err
		}
		doc, ok := state.Outputs[input.OutputID]
		if !ok || doc.Metadata.RunID != input.RunID {
			return errors.New("output not found")
		}
		dataPath := filepath.ToSlash(filepath.Dir(doc.Path)) + "/" + doc.Metadata.Name
		data, err := readRepositoryFile(root, dataPath)
		if err != nil {
			return err
		}
		digest := fmt.Sprintf("%x", sha256.Sum256(data))
		if digest != doc.Metadata.SHA256 || len(data) != doc.Metadata.Size {
			return errors.New("output integrity check failed")
		}
		content, encoding, err := encodeOutput(data, doc.Metadata.StoredEncode)
		if err != nil {
			return err
		}
		result = readOutputOutput{Name: doc.Metadata.Name, Content: content,
			Encoding: encoding, MediaType: doc.Metadata.MediaType, SHA256: digest, Size: len(data)}
		return nil
	})
	return result, err
}

func encodeOutput(data []byte, stored string) (string, string, error) {
	if stored == "base64" {
		return base64.StdEncoding.EncodeToString(data), "base64", nil
	}
	if stored != "utf-8" || !utf8.Valid(data) {
		return "", "", errors.New("stored output encoding is invalid")
	}
	return string(data), "utf-8", nil
}

func outputSearchText(root string, doc document) string {
	text := strings.Join([]string{doc.Metadata.Name, doc.Metadata.MediaType, strings.Join(doc.Metadata.ThreadIDs, " ")}, " ")
	dataPath := filepath.ToSlash(filepath.Dir(doc.Path)) + "/" + doc.Metadata.Name
	data, err := readRepositoryFile(root, dataPath)
	if err == nil && utf8.Valid(data) {
		text += " " + string(data)
	}
	return text
}
