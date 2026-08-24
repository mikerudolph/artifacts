package types

import "testing"

func TestParseScope(t *testing.T) {
	t.Parallel()
	cases := []struct {
		in      string
		want    Scope
		wantErr bool
	}{
		{"", ScopeWrite, false},
		{"write", ScopeWrite, false},
		{"read", ScopeRead, false},
		{"admin", "", true},
	}
	for _, tc := range cases {
		got, err := ParseScope(tc.in)
		if (err != nil) != tc.wantErr || got != tc.want {
			t.Fatalf("ParseScope(%q) = %q, %v", tc.in, got, err)
		}
	}
}

func TestParseTokenState(t *testing.T) {
	t.Parallel()
	cases := []struct {
		in      string
		want    TokenState
		wantErr bool
	}{
		{"active", TokenActive, false},
		{"expired", TokenExpired, false},
		{"revoked", TokenRevoked, false},
		{"", "", true},
		{"all", "", true},
	}
	for _, tc := range cases {
		got, err := ParseTokenState(tc.in)
		if (err != nil) != tc.wantErr || got != tc.want {
			t.Fatalf("ParseTokenState(%q) = %q, %v", tc.in, got, err)
		}
	}
	st, err := ParseTokenListState("")
	if err != nil || st != TokenActive {
		t.Fatalf("list default: %q %v", st, err)
	}
	st, err = ParseTokenListState("all")
	if err != nil || !IsTokenStateAll(st) {
		t.Fatalf("list all: %q %v", st, err)
	}
	if _, err := ParseTokenListState("nope"); err == nil {
		t.Fatal("expected list state error")
	}
	if IsTokenStateAll(TokenActive) {
		t.Fatal("active is not all")
	}
}

func TestParseRepoStatus(t *testing.T) {
	t.Parallel()
	for _, s := range []string{"ready", "importing", "forking", "deleting"} {
		got, err := ParseRepoStatus(s)
		if err != nil || string(got) != s {
			t.Fatalf("ParseRepoStatus(%q) = %q, %v", s, got, err)
		}
	}
	if _, err := ParseRepoStatus("bogus"); err == nil {
		t.Fatal("expected error")
	}
}

func TestParseJurisdiction(t *testing.T) {
	t.Parallel()
	got, err := ParseJurisdiction("")
	if err != nil || got != "" {
		t.Fatalf("empty: %q %v", got, err)
	}
	got, err = ParseJurisdiction("eu")
	if err != nil || got != JurisdictionEU {
		t.Fatalf("eu: %q %v", got, err)
	}
	got, err = ParseJurisdiction("us")
	if err != nil || got != JurisdictionUS {
		t.Fatalf("us: %q %v", got, err)
	}
	if _, err := ParseJurisdiction("apac"); err == nil {
		t.Fatal("expected error")
	}
}

func TestParseSort(t *testing.T) {
	t.Parallel()
	got, err := ParseRepoSortField("")
	if err != nil || got != SortCreatedAt {
		t.Fatalf("default sort: %q %v", got, err)
	}
	for _, s := range []string{"created_at", "updated_at", "last_push_at", "name"} {
		got, err := ParseRepoSortField(s)
		if err != nil || string(got) != s {
			t.Fatalf("sort %q: %q %v", s, got, err)
		}
	}
	if _, err := ParseRepoSortField("size"); err == nil {
		t.Fatal("expected error")
	}
	dir, err := ParseSortDirection("")
	if err != nil || dir != SortDesc {
		t.Fatalf("default dir: %q %v", dir, err)
	}
	dir, err = ParseSortDirection("asc")
	if err != nil || dir != SortAsc {
		t.Fatalf("asc: %q %v", dir, err)
	}
	if _, err := ParseSortDirection("sideways"); err == nil {
		t.Fatal("expected error")
	}
}
