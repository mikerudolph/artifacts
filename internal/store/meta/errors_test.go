package meta

import (
	"errors"
	"testing"
)

func TestErrorHelpers(t *testing.T) {
	t.Parallel()
	if !IsNotFound(ErrNotFound) || IsNotFound(ErrAlreadyExists) {
		t.Fatal("not found")
	}
	if !IsAlreadyExists(ErrAlreadyExists) || IsAlreadyExists(ErrNotFound) {
		t.Fatal("already exists")
	}
	if !IsCASConflict(ErrCASConflict) || IsCASConflict(errors.New("x")) {
		t.Fatal("cas")
	}
}
