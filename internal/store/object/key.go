package object

import (
	"fmt"
	"path"
	"strings"
)

func ValidateKey(key string) error {
	if key == "" || strings.HasPrefix(key, "/") || strings.Contains(key, "..") {
		return ErrInvalidKey
	}
	return nil
}

func ObjectsPrefix(accountID, repoID string) string {
	return path.Join(accountID, repoID)
}

func LooseObjectKey(accountID, repoID, sha string) string {
	sha = strings.ToLower(sha)
	if len(sha) < 2 {
		return path.Join(ObjectsPrefix(accountID, repoID), "objects", sha)
	}
	return path.Join(ObjectsPrefix(accountID, repoID), "objects", sha[:2], sha[2:])
}

func PackKey(accountID, repoID, name string) string {
	return path.Join(ObjectsPrefix(accountID, repoID), "pack", name+".pack")
}

func PackIndexKey(accountID, repoID, name string) string {
	return path.Join(ObjectsPrefix(accountID, repoID), "pack", name+".idx")
}

func RepoPrefix(accountID, repoID string) string {
	return ObjectsPrefix(accountID, repoID) + "/"
}

func FormatSHA(sha string) (string, error) {
	sha = strings.ToLower(strings.TrimSpace(sha))
	if len(sha) != 40 {
		return "", fmt.Errorf("%w: sha must be 40 hex chars", ErrInvalidKey)
	}
	for _, c := range sha {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return "", fmt.Errorf("%w: sha must be 40 hex chars", ErrInvalidKey)
		}
	}
	return sha, nil
}
