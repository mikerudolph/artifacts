package types

import "errors"

var (
	ErrInvalidName = errors.New("invalid name")

	ErrInvalidTTL = errors.New("invalid ttl")

	ErrInvalidScope = errors.New("invalid scope")

	ErrInvalidState = errors.New("invalid state")

	ErrInvalidSort = errors.New("invalid sort")

	ErrInvalidStatus = errors.New("invalid status")

	ErrInvalidJurisdiction = errors.New("invalid jurisdiction")
)
