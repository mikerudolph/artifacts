package meta

import "errors"

var (
	ErrNotFound = errors.New("not found")

	ErrAlreadyExists = errors.New("already exists")

	ErrCASConflict = errors.New("cas conflict")
)

func IsNotFound(err error) bool { return errors.Is(err, ErrNotFound) }

func IsAlreadyExists(err error) bool { return errors.Is(err, ErrAlreadyExists) }

func IsCASConflict(err error) bool { return errors.Is(err, ErrCASConflict) }
