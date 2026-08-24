package types

import "errors"

var (
	// ErrInvalidName is returned when a namespace or repo name is illegal.
	ErrInvalidName = errors.New("invalid name")
	// ErrInvalidTTL is returned when a token TTL is outside 60–31536000.
	ErrInvalidTTL = errors.New("invalid ttl")
	// ErrInvalidScope is returned when a token scope is not read or write.
	ErrInvalidScope = errors.New("invalid scope")
	// ErrInvalidState is returned when a token state filter is unknown.
	ErrInvalidState = errors.New("invalid state")
	// ErrInvalidSort is returned when a list sort field or direction is unknown.
	ErrInvalidSort = errors.New("invalid sort")
	// ErrInvalidStatus is returned when a repo status is unknown.
	ErrInvalidStatus = errors.New("invalid status")
	// ErrInvalidJurisdiction is returned when a jurisdiction is not eu, us, or empty.
	ErrInvalidJurisdiction = errors.New("invalid jurisdiction")
)
