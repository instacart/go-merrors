package merrors

import (
	stderrors "errors"
	"fmt"
)

// errorImpl is the default Error implementation.
type errorImpl struct {
	BaseError
}

// New creates a new error with message and captures a stack.
func New(message string) error {
	return &errorImpl{BaseError: NewBaseError(WithMessage(message))}
}

// Errorf creates an error with a formatted message, preserving wrapped causes.
func Errorf(format string, a ...any) error {
	raw := fmt.Errorf(format, a...)
	opts := []Option{WithMessage(raw.Error())}
	if multi, ok := raw.(interface{ Unwrap() []error }); ok {
		if joined := stderrors.Join(multi.Unwrap()...); joined != nil {
			opts = append(opts, Wrapping(joined), OmitWrappedMessage())
		}
	} else if single, ok := raw.(interface{ Unwrap() error }); ok {
		if cause := single.Unwrap(); cause != nil {
			opts = append(opts, Wrapping(cause), OmitWrappedMessage())
		}
	}
	return &errorImpl{BaseError: NewBaseError(opts...)}
}

// Wrap wraps err, applying options such as WithMessage or WithMessagef. With
// no options and an existing non-empty stack, Wrap returns err unchanged.
func Wrap(err error, opts ...Option) error {
	if err == nil {
		return nil
	}
	if len(opts) == 0 {
		if existing, ok := err.(Error); ok {
			stack := existing.merrorsBase().stack
			if stack != nil && len(stack.StackFrames()) > 0 {
				return existing
			}
		}
	}

	allOpts := make([]Option, 0, len(opts)+1)
	allOpts = append(allOpts, Wrapping(err))
	allOpts = append(allOpts, opts...)
	return &errorImpl{BaseError: NewBaseError(allOpts...)}
}

// NewSentinel creates a stackless error.
func NewSentinel(message string) error {
	return &errorImpl{BaseError: NewBaseError(WithMessage(message), WithoutStack())}
}

// As forwards to errors.As.
func As(err error, target any) bool { return stderrors.As(err, target) }

// Is forwards to errors.Is.
func Is(err, target error) bool { return stderrors.Is(err, target) }

// Unwrap forwards to errors.Unwrap.
func Unwrap(err error) error { return stderrors.Unwrap(err) }
