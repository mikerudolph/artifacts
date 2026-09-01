package main

import (
	"bytes"
	"encoding/base64"
	"strings"
	"testing"
	"time"
)

func TestTemporalValidation(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		value temporal
		ok    bool
	}{
		{"empty", temporal{}, true},
		{"valid", temporal{StartsAt: "2026-09-01T10:00:00Z", EndsAt: "2026-09-01T11:00:00Z", DueAt: "2026-09-02T10:00:00Z", Timezone: "America/Halifax"}, true},
		{"bad start", temporal{StartsAt: "Tuesday"}, false},
		{"bad end", temporal{EndsAt: "Tuesday"}, false},
		{"bad due", temporal{DueAt: "soon"}, false},
		{"reversed", temporal{StartsAt: "2026-09-02T10:00:00Z", EndsAt: "2026-09-01T10:00:00Z"}, false},
		{"bad timezone", temporal{Timezone: "Mars/Olympus"}, false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := validateTemporal(test.value); (err == nil) != test.ok {
				t.Fatalf("validateTemporal() error=%v ok=%v", err, test.ok)
			}
		})
	}
}

func TestNamesTagsEnumsAndIDs(t *testing.T) {
	t.Parallel()
	tags, err := normalizeTags([]string{" Work ", "work", "Review"})
	if err != nil || strings.Join(tags, ",") != "work,review" {
		t.Fatalf("tags=%v error=%v", tags, err)
	}
	if _, err := normalizeTags([]string{" "}); err == nil {
		t.Fatal("empty tag accepted")
	}
	if validateEnum("note", "kind", memoryKinds) != nil || validateEnum("other", "kind", memoryKinds) == nil {
		t.Fatal("enum validation")
	}
	for _, test := range []struct {
		id, prefix string
		ok         bool
	}{
		{"mem_abc123", "mem_", true}, {"mem_", "mem_", false},
		{"run_UPPER", "run_", false}, {"thread_abc", "mem_", false},
	} {
		if err := validateID(test.id, test.prefix); (err == nil) != test.ok {
			t.Fatalf("validateID(%q)=%v", test.id, err)
		}
	}
}

func TestOutputValidation(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"", ".", "..", "a/b", `a\b`, ".git", "bad\nname"} {
		if err := validateOutputName(name); err == nil {
			t.Errorf("unsafe name accepted: %q", name)
		}
	}
	if err := validateOutputName("report.md"); err != nil {
		t.Fatal(err)
	}
	if err := validateOutputName(".gitignore"); err != nil {
		t.Fatal(err)
	}
	plain, encoding, err := decodeOutput("hello", "")
	if err != nil || string(plain) != "hello" || encoding != "utf-8" {
		t.Fatalf("plain=%q encoding=%s error=%v", plain, encoding, err)
	}
	raw := []byte{0, 1, 0xff}
	decoded, encoding, err := decodeOutput(base64.StdEncoding.EncodeToString(raw), "base64")
	if err != nil || !bytes.Equal(decoded, raw) || encoding != "base64" {
		t.Fatalf("decoded=%v encoding=%s error=%v", decoded, encoding, err)
	}
	for _, test := range []struct{ content, encoding string }{{"x", "hex"}, {"%%%", "base64"}, {strings.Repeat("x", maxOutputBytes+1), "utf-8"}} {
		if _, _, err := decodeOutput(test.content, test.encoding); err == nil {
			t.Errorf("invalid output accepted: %s", test.encoding)
		}
	}
	tooLarge := base64.StdEncoding.EncodeToString(make([]byte, maxOutputBytes+1))
	if _, _, err := decodeOutput(tooLarge, "base64"); err == nil {
		t.Fatal("oversized base64 accepted")
	}
}

func TestRecordRoundTripAndErrors(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 8, 30, 12, 0, 0, 0, time.UTC)
	id, err := newID("mem_", now, bytes.NewReader(bytes.Repeat([]byte{1}, 8)))
	if err != nil || validateID(id, "mem_") != nil {
		t.Fatalf("id=%q error=%v", id, err)
	}
	if _, err := newID("mem_", now, bytes.NewReader(nil)); err == nil {
		t.Fatal("short randomness accepted")
	}
	generated, err := generateID("run_", now)
	if err != nil || validateID(generated, "run_") != nil {
		t.Fatalf("generated=%q error=%v", generated, err)
	}
	doc := document{Metadata: metadata{ID: id, Type: "memory", Kind: "note", RecordedAt: recordedAt(now)}, Content: "exact body\n"}
	data, err := renderDocument(doc)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := parseDocument(data)
	if err != nil || parsed.Metadata.ID != id || parsed.Content != doc.Content {
		t.Fatalf("parsed=%+v error=%v", parsed, err)
	}
	for _, data := range [][]byte{[]byte("body"), []byte("---\nid: x\n"), []byte("---\n:\n---\n"), []byte("---\nid: x\ntype: memory\n---\n")} {
		if _, err := parseDocument(data); err == nil {
			t.Errorf("invalid document accepted: %q", data)
		}
	}
	if cloneStrings(nil) != nil {
		t.Fatal("nil string slice was not preserved")
	}
}
