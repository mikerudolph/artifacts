package meta

import "errors"

var (
	// ErrNotFound means the row does not exist.
	ErrNotFound = errors.New("not found")
	// ErrAlreadyExists means a unique constraint was violated.
	ErrAlreadyExists = errors.New("already exists")
	// ErrCASConflict means a ref compare-and-swap did not match.
	ErrCASConflict = errors.New("cas conflict")
)

// IsNotFound reports whether err is ErrNotFound.
func IsNotFound(err error) bool { return errors.Is(err, ErrNotFound) }

// IsAlreadyExists reports whether err is ErrAlreadyExists.
func IsAlreadyExists(err error) bool { return errors.Is(err, ErrAlreadyExists) }

// IsCASConflict reports whether err is ErrCASConflict.
func IsCASConflict(err error) bool { return errors.Is(err, ErrCASConflict) }
