package main

import (
	"errors"
	"net"
)

const (
	classNone           = "none"
	classUnreachable    = "unreachable"
	classAuthentication = "authentication"
	classMalformed      = "unexpected_response"
	classAssertion      = "assertion"
	classProduct        = "product"
	classCleanup        = "cleanup"
	classEvidence       = "evidence"
)

type classifiedError struct {
	class string
	err   error
}

func (e *classifiedError) Error() string { return e.err.Error() }
func (e *classifiedError) Unwrap() error { return e.err }

func classified(class, message string) error {
	return &classifiedError{class: class, err: errors.New(message)}
}

func classify(err error) string {
	if err == nil {
		return classNone
	}
	var marked *classifiedError
	if errors.As(err, &marked) {
		return marked.class
	}
	var response *responseError
	if errors.As(err, &response) {
		if response.status == 401 || response.status == 403 {
			return classAuthentication
		}
		if response.malformed {
			return classMalformed
		}
		return classProduct
	}
	var netErr net.Error
	if errors.As(err, &netErr) {
		return classUnreachable
	}
	return classProduct
}
