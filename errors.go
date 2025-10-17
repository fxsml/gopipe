package gopipe

import (
	"errors"
)

// IsCancel checks if an error was caused by cancellation.
func IsCancel(err error) bool {
	return !IsFailure(err)
}

type errFailure struct {
	cause error
}

func (e *errFailure) Error() string {
	if e.cause != nil {
		return e.cause.Error()
	}
	return "gopipe: cause is nil"
}

func (e *errFailure) Unwrap() error {
	return e.cause
}

func newErrFailure(err error) error {
	return &errFailure{cause: err}
}

// IsFailure checks if an error was caused by a processing failure.
func IsFailure(err error) bool {
	var failureErr *errFailure
	return errors.As(err, &failureErr)
}
