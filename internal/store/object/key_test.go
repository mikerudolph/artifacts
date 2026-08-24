package object

import "testing"

func TestValidateKey(t *testing.T) {
	t.Parallel()
	if err := ValidateKey("a/b"); err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"", "/abs", "a/../b", ".."} {
		if err := ValidateKey(k); err == nil {
			t.Fatalf("expected invalid: %q", k)
		}
	}
}

func TestKeys(t *testing.T) {
	t.Parallel()
	if got := ObjectsPrefix("acct", "repo_1"); got != "acct/repo_1" {
		t.Fatal(got)
	}
	sha := "0123456789abcdef0123456789abcdef01234567"
	got := LooseObjectKey("acct", "repo_1", sha)
	want := "acct/repo_1/objects/01/23456789abcdef0123456789abcdef01234567"
	if got != want {
		t.Fatalf("loose %q", got)
	}
	if got := LooseObjectKey("a", "r", "x"); got != "a/r/objects/x" {
		t.Fatalf("short sha %q", got)
	}
	if got := PackKey("a", "r", "pack-1"); got != "a/r/pack/pack-1.pack" {
		t.Fatal(got)
	}
	if got := PackIndexKey("a", "r", "pack-1"); got != "a/r/pack/pack-1.idx" {
		t.Fatal(got)
	}
	if got := RepoPrefix("a", "r"); got != "a/r/" {
		t.Fatal(got)
	}
}

func TestFormatSHA(t *testing.T) {
	t.Parallel()
	sha := "0123456789ABCDEF0123456789abcdef01234567"
	got, err := FormatSHA(" " + sha + " ")
	if err != nil {
		t.Fatal(err)
	}
	if got != "0123456789abcdef0123456789abcdef01234567" {
		t.Fatal(got)
	}
	for _, bad := range []string{"", "abc", "gggggggggggggggggggggggggggggggggggggggg", "0123456789abcdef0123456789abcdef0123456"} {
		if _, err := FormatSHA(bad); err == nil {
			t.Fatalf("expected error for %q", bad)
		}
	}
}

func TestIsNotFound(t *testing.T) {
	t.Parallel()
	if !IsNotFound(ErrNotFound) {
		t.Fatal("expected match")
	}
	if IsNotFound(ErrInvalidKey) {
		t.Fatal("did not expect match")
	}
}
