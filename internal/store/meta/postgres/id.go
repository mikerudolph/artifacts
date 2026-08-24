package postgres

import (
	"crypto/rand"
	"encoding/hex"

	"github.com/mikerudolph/artifacts/internal/types"
)

func newHex(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b)
}

func newID() string { return newHex(8) }

func newRepoID() types.RepoID { return types.RepoID("repo_" + newID()) }
