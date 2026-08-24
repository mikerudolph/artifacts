package auth

import (
	"strings"
	"testing"
	"time"

	"github.com/mikerudolph/artifacts/internal/types"
)

func TestMintParseVerify(t *testing.T) {
	t.Parallel()
	now := time.Unix(1_700_000_000, 0).UTC()
	plain, hash, id, exp, err := MintRepo(types.ScopeRead, time.Hour, now)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(plain, repoPrefix) || !strings.Contains(plain, "?expires=") {
		t.Fatalf("plaintext %q", plain)
	}
	if len(id) != idLen {
		t.Fatalf("id %q", id)
	}
	if exp.Unix() != now.Add(time.Hour).Unix() {
		t.Fatalf("expires %v", exp)
	}
	secret, gotExp, err := ParseRepo(plain)
	if err != nil || gotExp.Unix() != exp.Unix() || HashRepo(secret) != hash {
		t.Fatalf("parse %q %v %v", secret, gotExp, err)
	}
	if err := VerifyRepo(plain, hash, now); err != nil {
		t.Fatal(err)
	}
	if err := VerifyRepo(secret, hash, now); err != nil {
		t.Fatal(err)
	}
	if err := VerifyRepo(plain, hash, exp); err != ErrExpired {
		t.Fatalf("at expiry: %v", err)
	}
	if err := VerifyRepo(plain, HashRepo("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"), now); err != ErrMismatch {
		t.Fatalf("mismatch: %v", err)
	}
}

func TestParseRepoTable(t *testing.T) {
	t.Parallel()
	good := "art_v1_0123456789abcdef0123456789abcdef01234567?expires=1760000000"
	secret, exp, err := ParseRepo(good)
	if err != nil || secret != "0123456789abcdef0123456789abcdef01234567" || exp.Unix() != 1760000000 {
		t.Fatalf("%q %v %v", secret, exp, err)
	}
	secret, exp, err = ParseRepo("0123456789abcdef0123456789abcdef01234567")
	if err != nil || secret == "" || !exp.IsZero() {
		t.Fatalf("secret only: %q %v %v", secret, exp, err)
	}
	for _, bad := range []string{"", "art_v1_short", "art_v1_" + strings.Repeat("g", 40), "art_v1_" + strings.Repeat("a", 40) + "?expires=nope"} {
		if _, _, err := ParseRepo(bad); err != ErrInvalidToken {
			t.Fatalf("%q -> %v", bad, err)
		}
	}
}

func TestMintInvalidScope(t *testing.T) {
	t.Parallel()
	_, _, _, _, err := MintRepo("admin", time.Hour, time.Now())
	if err != types.ErrInvalidScope {
		t.Fatalf("%v", err)
	}
}

func TestAPIToken(t *testing.T) {
	t.Parallel()
	hash := HashAPI("secret")
	if err := VerifyAPI("secret", hash); err != nil {
		t.Fatal(err)
	}
	if err := VerifyAPI("other", hash); err != ErrMismatch {
		t.Fatalf("%v", err)
	}
}
