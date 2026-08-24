package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/mikerudolph/artifacts/internal/types"
)

const (
	repoPrefix = "art_v1_"
	secretLen  = 20 // 40 hex chars
	idLen      = 16
)

var (
	// ErrInvalidToken means the plaintext is malformed.
	ErrInvalidToken = errors.New("invalid token")
	// ErrExpired means the token's expires timestamp is in the past.
	ErrExpired = errors.New("token expired")
	// ErrMismatch means the token hash does not match.
	ErrMismatch = errors.New("token mismatch")
)

// MintRepo creates a repo-scoped token. Plaintext is art_v1_<40 hex>?expires=<unix>.
func MintRepo(scope types.Scope, ttl time.Duration, now time.Time) (plaintext, hash string, id types.TokenID, expires time.Time, err error) {
	if _, err := types.ParseScope(string(scope)); err != nil {
		return "", "", "", time.Time{}, err
	}
	secret := make([]byte, secretLen)
	if _, err := rand.Read(secret); err != nil {
		return "", "", "", time.Time{}, err
	}
	hexSecret := hex.EncodeToString(secret)
	expires = now.Add(ttl)
	plaintext = fmt.Sprintf("%s%s?expires=%d", repoPrefix, hexSecret, expires.Unix())
	return plaintext, HashRepo(hexSecret), types.TokenID(hexSecret[:idLen]), expires, nil
}

// ParseRepo accepts the full art_v1_…?expires=… string or the secret-only form.
func ParseRepo(plaintext string) (secret string, expires time.Time, err error) {
	plaintext = strings.TrimSpace(plaintext)
	var expPart string
	if i := strings.Index(plaintext, "?expires="); i >= 0 {
		expPart = plaintext[i+len("?expires="):]
		plaintext = plaintext[:i]
	}
	secret, err = secretFrom(plaintext)
	if err != nil {
		return "", time.Time{}, err
	}
	if expPart == "" {
		return secret, time.Time{}, nil
	}
	sec, err := strconv.ParseInt(expPart, 10, 64)
	if err != nil {
		return "", time.Time{}, ErrInvalidToken
	}
	return secret, time.Unix(sec, 0).UTC(), nil
}

func secretFrom(s string) (string, error) {
	s = strings.TrimPrefix(s, repoPrefix)
	if len(s) != 40 {
		return "", ErrInvalidToken
	}
	for _, c := range s {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return "", ErrInvalidToken
		}
	}
	return s, nil
}

// HashRepo returns hex(sha256(secret)).
func HashRepo(secret string) string {
	sum := sha256.Sum256([]byte(secret))
	return hex.EncodeToString(sum[:])
}

// VerifyRepo checks plaintext against a stored hash and expiry.
func VerifyRepo(plaintext, storedHash string, now time.Time) error {
	secret, expires, err := ParseRepo(plaintext)
	if err != nil {
		return err
	}
	if !expires.IsZero() && !expires.After(now) {
		return ErrExpired
	}
	if subtle.ConstantTimeCompare([]byte(HashRepo(secret)), []byte(storedHash)) != 1 {
		return ErrMismatch
	}
	return nil
}

// HashAPI returns hex(sha256(plaintext)) for a control-plane token.
func HashAPI(plaintext string) string {
	sum := sha256.Sum256([]byte(plaintext))
	return hex.EncodeToString(sum[:])
}

// VerifyAPI compares a control-plane token to a stored hash.
func VerifyAPI(plaintext, storedHash string) error {
	if subtle.ConstantTimeCompare([]byte(HashAPI(plaintext)), []byte(storedHash)) != 1 {
		return ErrMismatch
	}
	return nil
}
