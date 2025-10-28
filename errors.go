package gopipe

import (
	"errors"
	"fmt"
)

var (
	// ErrFailure indicates a processing failure.
	ErrFailure = errors.New("gopipe: processing failed")
	// ErrCancel indicates that processing was canceled.
	ErrCancel = errors.New("gopipe: processing canceled")
)

type errFailure struct {
	cause error
}

func (e *errFailure) Error() string {
	if e.cause != nil {
		return e.cause.Error()
	}
	return ErrFailure.Error()
}

func (e *errFailure) Unwrap() error {
	return fmt.Errorf("%w: %w", ErrFailure, e.cause)
}

func newErrFailure(err error) error {
	return &errFailure{cause: err}
}

type errCancel struct {
	cause error
}

func (e *errCancel) Error() string {
	if e.cause != nil {
		return e.cause.Error()
	}
	return ErrCancel.Error()
}

func (e *errCancel) Unwrap() error {
	return fmt.Errorf("%w: %w", ErrCancel, e.cause)
}

func newErrCancel(err error) error {
	return &errCancel{cause: err}
}
