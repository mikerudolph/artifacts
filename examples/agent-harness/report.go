package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type report struct {
	RunID          string              `json:"run_id"`
	StartedAt      time.Time           `json:"started_at"`
	FinishedAt     time.Time           `json:"finished_at"`
	Configuration  reportConfiguration `json:"configuration"`
	Classification string              `json:"classification"`
	Failure        string              `json:"failure,omitempty"`
	Steps          []stepOutcome       `json:"steps"`
	Assertions     []assertionOutcome  `json:"assertions"`
	Artifacts      []artifactDigest    `json:"artifacts"`
}

type reportConfiguration struct {
	URL      string `json:"url"`
	Account  string `json:"account"`
	APIToken string `json:"api_token"`
}

type stepOutcome struct {
	Name       string `json:"name"`
	Success    bool   `json:"success"`
	DurationMS int64  `json:"duration_ms"`
	Detail     string `json:"detail,omitempty"`
}

type assertionOutcome struct {
	Name   string `json:"name"`
	Passed bool   `json:"passed"`
	Detail string `json:"detail,omitempty"`
}

type artifactDigest struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
	Size   int64  `json:"size"`
}

type recorder struct {
	report  report
	secrets []string
	gitLog  strings.Builder
}

func newRecorder(runID string, cfg configuration) *recorder {
	token := "<unset>"
	if cfg.token != "" {
		token = "<redacted>"
	}
	return &recorder{report: report{
		RunID: runID, StartedAt: time.Now().UTC(), Classification: classNone,
		Configuration: reportConfiguration{URL: redactURL(cfg.root), Account: cfg.account, APIToken: token},
		Steps:         []stepOutcome{}, Assertions: []assertionOutcome{}, Artifacts: []artifactDigest{},
	}, secrets: []string{cfg.token}}
}

func (r *recorder) step(name string, run func() (string, error)) error {
	started := time.Now()
	detail, err := run()
	r.report.Steps = append(r.report.Steps, stepOutcome{
		Name: name, Success: err == nil, DurationMS: time.Since(started).Milliseconds(), Detail: r.redact(detail),
	})
	return err
}

func (r *recorder) assert(name string, passed bool, detail string) error {
	r.report.Assertions = append(r.report.Assertions, assertionOutcome{Name: name, Passed: passed, Detail: r.redact(detail)})
	if !passed {
		return classified(classAssertion, name)
	}
	return nil
}

func (r *recorder) addSecret(secret string) {
	if secret != "" {
		r.secrets = append(r.secrets, secret)
		if value, _, found := strings.Cut(secret, "?"); found {
			r.secrets = append(r.secrets, value)
		}
	}
}

func (r *recorder) redact(value string) string {
	for _, secret := range r.secrets {
		if secret != "" {
			value = strings.ReplaceAll(value, secret, "<redacted>")
		}
	}
	return value
}

func (r *recorder) finish(err error) {
	r.report.FinishedAt = time.Now().UTC()
	r.report.Classification = classify(err)
	if err != nil {
		r.report.Failure = r.redact(err.Error())
	}
}

func (r *recorder) write(evidence string) error {
	if err := os.MkdirAll(evidence, 0o700); err != nil {
		return err
	}
	gitData := []byte(r.redact(r.gitLog.String()))
	if err := os.WriteFile(filepath.Join(evidence, "git.log"), gitData, 0o600); err != nil {
		return err
	}
	digest := sha256.Sum256(gitData)
	r.report.Artifacts = append(r.report.Artifacts, artifactDigest{
		Path: "git.log", SHA256: hex.EncodeToString(digest[:]), Size: int64(len(gitData)),
	})
	data, err := json.MarshalIndent(r.report, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(evidence, "report.json"), append(data, '\n'), 0o600); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(evidence, "report.md"), []byte(renderMarkdown(r.report)), 0o600)
}

func redactURL(raw string) string {
	parsed, err := url.Parse(raw)
	if err != nil {
		return "<invalid>"
	}
	parsed.User = nil
	query := parsed.Query()
	for key := range query {
		lower := strings.ToLower(key)
		if strings.Contains(lower, "token") || strings.Contains(lower, "key") ||
			strings.Contains(lower, "secret") || strings.Contains(lower, "password") {
			query.Set(key, "<redacted>")
		}
	}
	parsed.RawQuery = query.Encode()
	return parsed.String()
}

func renderMarkdown(got report) string {
	var out strings.Builder
	_, _ = fmt.Fprintf(&out, "# Artifacts verification %s\n\nClassification: `%s`\n\n", got.RunID, got.Classification)
	if got.Failure != "" {
		_, _ = fmt.Fprintf(&out, "Failure: %s\n\n", got.Failure)
	}
	out.WriteString("## Steps\n\n")
	for _, step := range got.Steps {
		_, _ = fmt.Fprintf(&out, "- [%s] %s (%d ms) %s\n", mark(step.Success), step.Name, step.DurationMS, step.Detail)
	}
	out.WriteString("\n## Assertions\n\n")
	for _, check := range got.Assertions {
		_, _ = fmt.Fprintf(&out, "- [%s] %s %s\n", mark(check.Passed), check.Name, check.Detail)
	}
	out.WriteString("\n## Artifacts\n\n")
	artifacts := append([]artifactDigest(nil), got.Artifacts...)
	sort.Slice(artifacts, func(i, j int) bool { return artifacts[i].Path < artifacts[j].Path })
	for _, artifact := range artifacts {
		_, _ = fmt.Fprintf(&out, "- `%s` `%s` (%d bytes)\n", artifact.Path, artifact.SHA256, artifact.Size)
	}
	return out.String()
}

func mark(ok bool) string {
	if ok {
		return "x"
	}
	return " "
}
