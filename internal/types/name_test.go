package types

import "testing"

func TestParseNamespaceName(t *testing.T) {
	t.Parallel()
	cases := []struct {
		in      string
		wantErr bool
	}{
		{"default", false},
		{"prod", false},
		{"a", false},
		{"1abc", false},
		{"agents-realtime", false},
		{"foo.bar_baz-1", false},
		{"", true},
		{"-bad", true},
		{".bad", true},
		{"_bad", true},
		{"has space", true},
		{"has/slash", true},
		{string(make([]byte, maxNameLen+1)), true},
	}
	for _, tc := range cases {
		_, err := ParseNamespaceName(tc.in)
		if (err != nil) != tc.wantErr {
			t.Fatalf("ParseNamespaceName(%q) err=%v wantErr=%v", tc.in, err, tc.wantErr)
		}
	}
}

func TestParseRepoName(t *testing.T) {
	t.Parallel()
	got, err := ParseRepoName("starter-repo")
	if err != nil || got != "starter-repo" {
		t.Fatalf("got %q err %v", got, err)
	}
	if _, err := ParseRepoName(""); err == nil {
		t.Fatal("expected error")
	}
}

func TestParseBranchName(t *testing.T) {
	t.Parallel()
	got, err := ParseBranchName("main")
	if err != nil || got != "main" {
		t.Fatalf("got %q err %v", got, err)
	}
	if _, err := ParseBranchName(""); err == nil {
		t.Fatal("expected error")
	}
	if _, err := ParseBranchName(string(make([]byte, maxNameLen+1))); err == nil {
		t.Fatal("expected error")
	}
}

func TestValidateNameMaxLen(t *testing.T) {
	t.Parallel()
	ok := make([]byte, maxNameLen)
	for i := range ok {
		ok[i] = 'a'
	}
	if _, err := ParseRepoName(string(ok)); err != nil {
		t.Fatalf("max len should pass: %v", err)
	}
}
