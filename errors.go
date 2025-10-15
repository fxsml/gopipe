package gopipe

import (
	"errors"
)

// ErrCancel represents an error that occurred due to cancellation.
// It wraps the original error while maintaining the original error message.
type ErrCancel struct {
	cause error
}

func (e *ErrCancel) Error() string {
	if e.cause != nil {
		return e.cause.Error()
	}
	return "gopipe canceled"
}

func (e *ErrCancel) Unwrap() error {
	return e.cause
}

func newErrCancel(err error) error {
	return &ErrCancel{cause: err}
}

// IsCancel checks if an error is or wraps an ErrCancel.
func IsCancel(err error) bool {
	var cancelErr *ErrCancel
	return errors.As(err, &cancelErr)
}

// ErrFailure represents an error that occurred due to operation failure.
// It wraps the original error while maintaining the original error message.
type ErrFailure struct {
	cause error
}

func (e *ErrFailure) Error() string {
	if e.cause != nil {
		return e.cause.Error()
	}
	return "gopipe failed"
}

func (e *ErrFailure) Unwrap() error {
	return e.cause
}

func newErrFailure(err error) error {
	return &ErrFailure{cause: err}
}

// IsFailure checks if an error is or wraps an ErrFailure.
func IsFailure(err error) bool {
	var failureErr *ErrFailure
	return errors.As(err, &failureErr)
}
