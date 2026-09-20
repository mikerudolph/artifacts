package types

import "errors"

var (
	ErrForbidden           = errors.New("access is read only")
	ErrIdempotencyConflict = errors.New("idempotency key was already used with different input")
)

type InputError struct {
	Field    string
	Message  string
	TooLarge bool
}

func (e *InputError) Error() string { return e.Message }

type HeadConflict struct{ Current string }

func (e *HeadConflict) Error() string { return "branch head changed; reconcile before publishing" }
