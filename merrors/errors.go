package merrors

import (
	stderrors "errors"
	"fmt"
	"io"
	"strings"

	"github.com/go-errors/errors"
)

type hasStackFrames interface {
	error
	StackFrames() []errors.StackFrame
}

type hasMessage interface {
	Message() string
}

func ConcatMessage(prefix string, suffix string, separator string) string {
	if prefix != "" && suffix != "" {
		return prefix + separator + suffix
	} else {
		return prefix + suffix
	}
}

// Represents an Error with a stacktrace.
//
// This is a wrapper around the go-errors package.
//
// This type can be embedded for custom error types that want to
// have the same behavior. Those types should also create
// a builder extending BaseErrorBuilder.
type BaseError[T any] struct {
	self        EmbedsBaseError[T]
	message     string
	errForStack hasStackFrames
	createError func() T
	wrappedErr  error

	// originalErr is set on a stack-less clone created by WithoutStack. It
	// lets errors.Is keep matching the error the clone replaced.
	originalErr error
}

func (e *BaseError[T]) baseError() *BaseError[T] { //nolint:unused
	return e
}

func (e *BaseError[T]) genericBaseError() Error { //nolint:unused
	return e
}

func (e *BaseError[T]) Clone() Error {
	clone := e.createError()
	e.self.PopulateClone(clone)
	return any(clone).(Error)
}

func (e *BaseError[T]) PopulateClone(clone T) {
	embedsBaseClone := any(clone).(EmbedsBaseError[T])
	baseClone := embedsBaseClone.baseError()
	baseClone.createError = e.createError
	baseClone.message = e.message
	baseClone.errForStack = e.errForStack
	baseClone.wrappedErr = e.wrappedErr
	baseClone.originalErr = e.originalErr
	baseClone.self = embedsBaseClone
}

func (c *BaseError[T]) WithMessageOverride(message string) Error {
	clone := c.Clone()
	baseClone := any(clone).(EmbedsBaseError[T]).baseError()
	baseClone.message = message
	return clone
}

func (c *BaseError[T]) WithAppendMessage(message string) Error {
	return c.WithMessageOverride(ConcatMessage(c.message, message, "\n"))
}

func (c *BaseError[T]) WithAppendMessagef(format string, a ...any) Error {
	return c.WithAppendMessage(fmt.Sprintf(format, a...))
}

func (c *BaseError[T]) WithoutStack() Error {
	clone := c.Clone()
	baseClone := any(clone).(EmbedsBaseError[T]).baseError()
	baseClone.errForStack = nil
	baseClone.originalErr = c.self
	return clone
}

// Returns just the error message without augmenting it.
//
// When this error is copied with Wrap, the Message() method will
// be called instead of Error() method. This allows you to add more details
// to the Error() method even if you don't want to propagate it as part of
// the message when copying error details in error handling.
func (e *BaseError[T]) Message() string {
	return e.message
}

// Error implements error
//
// You may want to override this method in your custom error types.
// Whenever we call this method internally we call your custom implementation.
func (e *BaseError[T]) Error() string {
	return e.Message()
}

// Unwrap returns the wrapped error.
func (e *BaseError[T]) Unwrap() error {
	return e.wrappedErr
}

// Is preserves matching against the error this stack-less clone replaced.
// It intentionally does not recursively search the original error's chain:
// Unwrap already walks that chain, and searching it here as well makes repeated
// WrapMessage calls exponential for an absent target.
func (e *BaseError[T]) Is(target error) bool {
	if e.originalErr == nil {
		return false
	}
	if target == e.originalErr {
		return true
	}
	if iser, ok := e.originalErr.(interface{ Is(error) bool }); ok {
		return iser.Is(target)
	}
	return false
}

func (e *BaseError[T]) Format(s fmt.State, verb rune) {
	switch verb {
	case 'v':
		if s.Flag('+') {
			io.WriteString(s, FormatErrorStack(e.self)) //nolint:errcheck
			return
		}
		fallthrough
	case 's':
		io.WriteString(s, e.self.Error()) //nolint:errcheck
	case 'q':
		fmt.Fprintf(s, "%q", e.self.Error()) //nolint:errcheck
	case 'x':
		fmt.Fprintf(s, "%x", e.self.Error()) //nolint:errcheck
	case 'X':
		fmt.Fprintf(s, "%X", e.self.Error()) //nolint:errcheck
	}
}

// FormatErrorStack returns the error message and
// first non empty stack trace formatted for logging purposes.
//
// The stack is searched for in err and then down its unwrap chain. Errors
// holding multiple children (errors.Join, fmt.Errorf with several %w verbs)
// are searched in child order, depth-first.
func FormatErrorStack(err error) string {
	var b strings.Builder
	b.WriteString(err.Error())
	for _, frame := range firstStackFrames(err) {
		b.WriteString(fmt.Sprintf("\n%s.%s\n\t%s:%d", frame.Package, frame.Name, frame.File, frame.LineNumber))
	}
	return b.String()
}

// firstStackFrames returns the first non empty stack found in err's unwrap
// tree, searching the children of multi errors (errors.Join, multiple %w
// verbs) in order.
func firstStackFrames(err error) []errors.StackFrame {
	for err != nil {
		if castErr, ok := err.(hasStackFrames); ok {
			if frames := castErr.StackFrames(); len(frames) > 0 {
				return frames
			}
		}
		switch unwrapped := err.(type) {
		case interface{ Unwrap() error }:
			err = unwrapped.Unwrap()
		case interface{ Unwrap() []error }:
			for _, child := range unwrapped.Unwrap() {
				if frames := firstStackFrames(child); len(frames) > 0 {
					return frames
				}
			}
			return nil
		default:
			return nil
		}
	}
	return nil
}

// hasStack reports whether a stack provider is attached without materializing
// its frames.
func (e *BaseError[T]) hasStack() bool {
	return e.errForStack != nil
}

func (e *BaseError[T]) StackFrames() []errors.StackFrame {
	// We allow some errors to omit the stack if they are in a chain of errors
	// and the stack would be repetitive.
	if e.errForStack == nil {
		return []errors.StackFrame{}
	}
	return e.errForStack.StackFrames()
}

type BaseErrorBuilder[B any, T EmbedsBaseError[T]] struct {
	self       B
	selfEmbeds EmbedsBaseErrorBuilder[B, T]

	createError        func() T
	message            string
	wrappedMessage     string
	errForStack        hasStackFrames
	wrappingNilErr     bool
	wrappedErr         error
	omitWrappedMessage bool
}

type EmbedsBaseErrorBuilder[B any, T EmbedsBaseError[T]] interface {
	// PopulateBuilderResult populates the builder result
	// with the data collected by the builder.
	//
	// It is called internally by Build() methods in subtypes.
	// This gives a chance for subtypes to populate the result or
	// to modify the state of the builder.
	//
	// subtypes overriding this method should call their supertype
	// implementation.
	PopulateBuilderResult(result T)

	baseErrorBuilder() *BaseErrorBuilder[B, T]
}

func (b *BaseErrorBuilder[B, T]) baseErrorBuilder() *BaseErrorBuilder[B, T] { //nolint:unused
	return b
}

// Wrap makes the built error wrap the given error.
//
// If the wrapped error is nil, then we will return nil when Build is called.
//
// This will effectively copy the stack and message from the wrapped
// error if they are available.
//
// The wrapped error is available via the errors.Unwrap method.
//
// This is useful for converting an error to a different type or
// adding more context to the error.
func (b *BaseErrorBuilder[B, T]) Wrap(wrappedErr error) B {
	// This call replaces the wrapped error; drop any stack selected for the
	// previous one before choosing the new replacement's stack.
	b.errForStack = nil
	if wrappedErr == nil {
		b.wrappingNilErr = true
		return b.self
	}
	b.wrappingNilErr = false
	b.wrappedErr = wrappedErr
	// If the wrapped error is a go-errors error, we can just unwrap it as
	// it is not a custom error type that people want to check against generally
	// speaking and it helps reduce the number of wrapped errors when reporting
	// to sentry.
	if goErr, ok := wrappedErr.(*errors.Error); ok {
		b.wrappedErr = errors.Unwrap(goErr)
	}
	// Only adopt the wrapped error's stack if it actually has frames. An
	// explicitly stack-less error (e.g. original.WithoutStack()) must not
	// suppress the stack captured by Build.
	if errWithStack, ok := wrappedErr.(hasStackFrames); ok && len(errWithStack.StackFrames()) > 0 {
		b.errForStack = errWithStack
	}
	if errWithStack, ok := wrappedErr.(Error); ok {
		b.wrappedErr = errWithStack.WithoutStack()
	}

	var msg string
	if errWithMsg, ok := wrappedErr.(hasMessage); ok {
		msg = errWithMsg.Message()
	} else {
		msg = wrappedErr.Error()
	}
	b.wrappedMessage = msg
	return b.self
}

// OmitWrappedMessage sets the builder to omit the message of the wrapped error
// and instead just use the message set with SetMessage/SetMessagef.
func (b *BaseErrorBuilder[B, T]) OmitWrappedMessage() B {
	b.omitWrappedMessage = true
	return b.self
}

// GetMessage returns the message set for this error with SetMessage/SetMessagef.
func (b *BaseErrorBuilder[B, T]) GetMessage() string {
	return b.message
}

// SetMessage sets the message for the error.
//
// Note: If Wrap is called, the message set here will be appended
// to the message of the wrapped error in a separate line.
func (b *BaseErrorBuilder[B, T]) SetMessage(message string) B {
	b.message = message
	return b.self
}

// SetMessagef is like SetMessage but with format arguments
func (b *BaseErrorBuilder[B, T]) SetMessagef(format string, a ...any) B {
	return b.SetMessage(fmt.Sprintf(format, a...))
}

func (b *BaseErrorBuilder[B, T]) Self() B {
	return b.self
}

func (b *BaseErrorBuilder[B, T]) PopulateBuilderResult(result T) {
	baseError := result.baseError()
	baseError.createError = b.createError
	if b.omitWrappedMessage {
		baseError.message = b.message
	} else {
		baseError.message = ConcatMessage(b.wrappedMessage, b.message, "\n")
	}
	baseError.errForStack = b.errForStack
	baseError.wrappedErr = b.wrappedErr
	baseError.self = result
}

// CaptureStack captures the current stack if we haven't already.
//
// This is meant to be called from within Build() methods in subtypes.
func (b *BaseErrorBuilder[B, T]) CaptureStack(skip int) B {
	if b.errForStack == nil {
		b.errForStack = errors.Wrap("", skip+1)
	}
	return b.self
}

// Build builds an instance of the error and returns it. A wrapped nil comes back as an
// untyped nil, so callers can return it from error-returning functions.
func (b *BaseErrorBuilder[B, T]) Build() Error {
	if b.wrappingNilErr {
		return nil
	}
	b.CaptureStack(1 /* skip */)
	result := b.createError()
	b.selfEmbeds.PopulateBuilderResult(result)
	// Calling clone here forces every subclass to also implement PopulateClone appropriately.
	return result.Clone()
}

// InitBaseErrorBuilder initializes the BaseErrorBuilder.
//
// This is a helper function to be used in the constructor of a
// custom error builders.
func InitBaseErrorBuilder[B EmbedsBaseErrorBuilder[B, T], T EmbedsBaseError[T]](self B, createError func() T) B {
	baseErrorBuilder := self.baseErrorBuilder()
	baseErrorBuilder.self = self
	baseErrorBuilder.selfEmbeds = self
	baseErrorBuilder.createError = createError
	return self
}

// Error is a common interface for all errors generated by this package.
type Error interface {
	error

	// Error returns the error message without augumentation.
	//
	// When this error is copied with Wrap, the Message() method will
	// be called instead of Error() method. This allows you to add more details
	// to the Error() method even if you don't want to propagate it as part of
	// the message when copying error details in error handling.
	Message() string
	// Clone returns a clone of the error.
	//
	// Generally this should not be called by users but it may be called by
	// subtypes that implement functions to return variations of an already built error
	// (like what the BaseError type does with WithAppendMessage).
	Clone() Error
	// WithMessageOverride returns a new error with the same type as the original error
	// but with a completely different message.
	WithMessageOverride(message string) Error
	// WithAppendMessage returns a new error with the same type as the original error
	// but with a message that is the original message plus the given message.
	WithAppendMessage(message string) Error
	// WithAppendMessagef is like WithAppendMessage but with formatting.
	WithAppendMessagef(format string, a ...any) Error
	// StackFrames returns the stack frames of the error.
	StackFrames() []errors.StackFrame
	// genericBaseError returns the error as a Error.
	//
	// This is there mainly to ensure only errors generated by this package
	// implement the interface.
	genericBaseError() Error
	// WithoutStack returns a new error with the same type as the original error
	// but with the stack omitted.
	WithoutStack() Error
}

// EmbedsBaseError is implemented by custom errors that embed BaseError.
type EmbedsBaseError[T any] interface {
	Error
	baseError() *BaseError[T]

	// Not meant to be called by users. Called internally by the pacakge
	// when cloning errors.
	//
	// It is called internally by Clone() methods in subtypes.
	// This gives a chance for subtypes to clone their custom fields.
	//
	// subtypes overriding this method should call their supertype
	// implementation.
	PopulateClone(clone T)
}

// errorImpl is the default Error implementation.
type errorImpl struct {
	BaseError[*errorImpl]
}

// ErrorBuilder builds the default Error implementation.
type ErrorBuilder struct {
	BaseErrorBuilder[*ErrorBuilder, *errorImpl]
}

// NewErrorBuilder creates a new ErrorBuilder
func NewErrorBuilder() *ErrorBuilder {
	return InitBaseErrorBuilder(&ErrorBuilder{}, func() *errorImpl { return &errorImpl{} })
}

// Errorf is a shorthand to create an error with a formatted message.
func Errorf(format string, a ...any) Error {
	raw := fmt.Errorf(format, a...)
	b := NewErrorBuilder()
	// fmt.Errorf returns an error with Unwrap() []error when the format has
	// multiple %w operands. errors.Unwrap only understands Unwrap() error, so
	// retain every cause explicitly or errors.Is/errors.As cannot reach them.
	if multi, ok := raw.(interface{ Unwrap() []error }); ok {
		if joined := stderrors.Join(multi.Unwrap()...); joined != nil {
			b.Wrap(joined).OmitWrappedMessage()
		}
	} else if single, ok := raw.(interface{ Unwrap() error }); ok {
		if cause := single.Unwrap(); cause != nil {
			b.Wrap(cause).OmitWrappedMessage()
		}
	}
	return b.SetMessage(raw.Error()).CaptureStack(1).Build()
}

// New creates a new error with the given message and captures the stack.
func New(message string) error {
	return NewErrorBuilder().SetMessage(message).CaptureStack(1).Build()
}

// Wraps the given error with stack trace
func Wrap(err error) Error {
	if err == nil {
		return nil
	}
	if existing, ok := err.(Error); ok {
		if withStack, ok := existing.(interface{ hasStack() bool }); ok {
			if withStack.hasStack() {
				return existing
			}
		} else if len(existing.StackFrames()) > 0 {
			return existing
		}
	}
	return NewErrorBuilder().Wrap(err).CaptureStack(1).Build()
}

// Wraps the given error with a new error message.
func WrapMessage(err error, message string) Error {
	if err == nil {
		return nil
	}
	return NewErrorBuilder().Wrap(err).SetMessage(message).CaptureStack(1).Build()
}

// Wraps the given error with a new error message using formatting
func WrapMessagef(err error, format string, a ...any) Error {
	if err == nil {
		return nil
	}
	return NewErrorBuilder().Wrap(err).SetMessagef(format, a...).CaptureStack(1).Build()
}

// NewSentinel creates a sentinel error without a stack.
//
// This is useful for defining package level error constants.
func NewSentinel(message string) error {
	return stderrors.New(message)
}

func As(err error, target any) bool {
	return stderrors.As(err, target)
}

func Unwrap(err error) error {
	return stderrors.Unwrap(err)
}
