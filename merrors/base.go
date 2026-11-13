package merrors

import (
	"fmt"
	"io"
	"slices"
	"strconv"
	"strings"
	"sync"

	goerrors "github.com/go-errors/errors"
)

type stackSource interface {
	StackFrames() []goerrors.StackFrame
}

type stackFramesProvider struct {
	source       stackSource
	frames       []goerrors.StackFrame
	cachedFrames bool
	once         sync.Once
}

func newStackFramesProvider(source stackSource) *stackFramesProvider {
	if source == nil {
		return nil
	}
	return &stackFramesProvider{source: source}
}

var externalStackFramesMu sync.Mutex

// cachedStackFrames wraps already-materialized frames without a lazy source.
func cachedStackFrames(frames []goerrors.StackFrame) *stackFramesProvider {
	if len(frames) == 0 {
		return nil
	}
	return &stackFramesProvider{frames: frames, cachedFrames: true}
}

// newExternalStackFramesProvider materializes an external source's frames and
// returns nil when the source is stackless. go-errors' first call is serialized
// because its lazy cache is not safe for concurrent initialization.
func newExternalStackFramesProvider(source stackSource) *stackFramesProvider {
	var frames []goerrors.StackFrame
	if goErr, ok := source.(*goerrors.Error); ok {
		externalStackFramesMu.Lock()
		frames = goErr.StackFrames()
		externalStackFramesMu.Unlock()
	} else {
		frames = source.StackFrames()
	}
	if len(frames) == 0 {
		return nil
	}
	return &stackFramesProvider{source: source, frames: frames, cachedFrames: true}
}

func (p *stackFramesProvider) StackFrames() []goerrors.StackFrame {
	if p == nil {
		return []goerrors.StackFrame{}
	}
	p.once.Do(func() {
		if p.cachedFrames {
			return
		}
		p.frames = p.source.StackFrames()
		p.cachedFrames = true
	})
	return p.frames
}

// Option configures NewBaseError.
type Option func(*config)

type config struct {
	message        string
	cause          error
	omitCause      bool
	combiner       MessageCombiner
	skip           int
	without        bool
	stackFrames    []goerrors.StackFrame
	stackFramesSet bool
}

// MessageCombiner combines a wrapped cause's message with the current error's
// message. Both arguments may be empty.
type MessageCombiner func(wrappedMessage, currentMessage string) string

// DefaultMessageCombiner returns the wrapped message first, then the current
// message on a new line. Empty messages are omitted.
func DefaultMessageCombiner(wrappedMessage, currentMessage string) string {
	switch {
	case wrappedMessage == "":
		return currentMessage
	case currentMessage == "":
		return wrappedMessage
	default:
		return wrappedMessage + "\n" + currentMessage
	}
}

// WithMessage sets this error's own message. The last message option wins.
func WithMessage(message string) Option {
	return func(c *config) { c.message = message }
}

// WithMessagef sets this error's own formatted message. The last message option wins.
func WithMessagef(format string, a ...any) Option {
	return func(c *config) { c.message = fmt.Sprintf(format, a...) }
}

// WithMessageCombiner sets how wrapped and current messages are combined.
// The last combiner option wins; nil restores DefaultMessageCombiner.
func WithMessageCombiner(combiner MessageCombiner) Option {
	return func(c *config) { c.combiner = combiner }
}

// Wrapping sets the cause of this error.
func Wrapping(err error) Option {
	return func(c *config) { c.cause = err }
}

// OmitWrappedMessage keeps only this error's own message.
func OmitWrappedMessage() Option {
	return func(c *config) { c.omitCause = true }
}

// SkipFrames skips extra frames when capturing a new stack.
func SkipFrames(n int) Option {
	return func(c *config) { c.skip += n }
}

// WithoutStack disables capturing a new stack.
func WithoutStack() Option {
	return func(c *config) { c.without = true }
}

// WithStackFrames supplies the stack explicitly. It is intended for custom
// constructors that need to provide frames and for panic recovery.
func WithStackFrames(frames []goerrors.StackFrame) Option {
	return func(c *config) {
		c.stackFrames = append([]goerrors.StackFrame(nil), frames...)
		c.stackFramesSet = true
	}
}

// BaseError holds the data shared by every merrors error. Embed it by value in
// custom error types.
//
// BaseError owns Error, Format, and StackFrames. Custom types must not
// implement those methods; doing so breaks agreement between err.Error(), %v,
// %+v, FormatErrorStack, and the captured stack. Render custom fields into the
// message when constructing the error, while keeping the fields on the struct
// when callers need them via errors.As.
type BaseError struct {
	message string
	cause   error
	stack   *stackFramesProvider
}

// Error is the common interface for merrors errors.
//
// Error() returns the composed message: wrapped cause first, then each
// wrapper's message on a new line. BaseError owns that formatting.
type Error interface {
	// Error returns the composed message.
	error
	// StackFrames returns the captured origin stack.
	StackFrames() []goerrors.StackFrame
	// merrorsBase exposes the embedded BaseError to the package.
	merrorsBase() *BaseError
}

// NewBaseError builds a BaseError. By default it captures the stack at the
// caller of the constructor that calls NewBaseError.
func NewBaseError(opts ...Option) BaseError {
	cfg := &config{}
	for _, opt := range opts {
		opt(cfg)
	}

	causeMessage := ""
	if cfg.cause != nil && !cfg.omitCause {
		causeMessage = cfg.cause.Error()
	}
	combiner := cfg.combiner
	if combiner == nil {
		combiner = DefaultMessageCombiner
	}
	message := combiner(causeMessage, cfg.message)

	var stack *stackFramesProvider
	switch {
	case cfg.stackFramesSet:
		stack = cachedStackFrames(cfg.stackFrames)
	case cfg.without:
		stack = nil
	default:
		stack = findStack(cfg.cause)
		if stack == nil {
			stack = newStackFramesProvider(captureStack(cfg.skip))
		}
	}

	return BaseError{message: message, cause: cfg.cause, stack: stack}
}

func findStack(err error) *stackFramesProvider {
	for err != nil {
		if merr, ok := err.(Error); ok {
			if stack := merr.merrorsBase().stack; stack != nil && len(stack.StackFrames()) > 0 {
				return stack
			}
		} else if source, ok := err.(stackSource); ok {
			if stack := newExternalStackFramesProvider(source); stack != nil {
				return stack
			}
		}
		switch e := err.(type) {
		case interface{ Unwrap() error }:
			err = e.Unwrap()
		case interface{ Unwrap() []error }:
			for _, child := range e.Unwrap() {
				if stack := findStack(child); stack != nil {
					return stack
				}
			}
			return nil
		default:
			return nil
		}
	}
	return nil
}

func captureStack(skip int) stackSource {
	return goerrors.Wrap("", skip+3)
}

func (b *BaseError) Error() string                      { return b.message }
func (b *BaseError) Unwrap() error                      { return b.cause }
func (b *BaseError) StackFrames() []goerrors.StackFrame { return b.stack.StackFrames() }
func (b *BaseError) merrorsBase() *BaseError            { return b }

// CheckErrorType reports an error if err overrides Error, StackFrames, or
// Format instead of relying on the embedded BaseError. Call it from tests of
// custom error types.
func CheckErrorType(err Error) error {
	if err == nil {
		return fmt.Errorf("merrors: nil error")
	}
	base := err.merrorsBase()
	if base == nil {
		return fmt.Errorf("merrors: %T has no BaseError", err)
	}
	if err.Error() != base.Error() {
		return fmt.Errorf("merrors: %T overrides Error(); embed BaseError and render custom fields into the message instead", err)
	}
	if !slices.Equal(err.StackFrames(), base.StackFrames()) {
		return fmt.Errorf("merrors: %T overrides StackFrames(); use WithStackFrames when constructing the error instead", err)
	}
	if fmt.Sprintf("%+v", err) != FormatErrorStack(err) {
		return fmt.Errorf("merrors: %T overrides Format(); embed BaseError and let it own formatting", err)
	}
	return nil
}

// CloneBaseErrorWithAppendMessage returns a copy of b with message appended
// on a new line, preserving its cause and stack.
//
// This is for advanced use: building a new error of the same custom type with
// an adjusted message. Most code should set the message at construction or add
// context with Wrap instead.
func CloneBaseErrorWithAppendMessage(b BaseError, message string) BaseError {
	clone := b
	if clone.message == "" {
		clone.message = message
	} else if message != "" {
		clone.message += "\n" + message
	}
	return clone
}

// Format implements fmt.Formatter. %+v appends the first stack in the chain;
// other verbs preserve BaseError string formatting flags.
func (b *BaseError) Format(s fmt.State, verb rune) {
	switch verb {
	case 'v':
		if s.Flag('+') {
			_, _ = io.WriteString(s, FormatErrorStack(b))
			return
		}
		writeFormattedString(s, 's', b.Error())
	case 's', 'q', 'x', 'X':
		writeFormattedString(s, verb, b.Error())
	}
}

func writeFormattedString(s fmt.State, verb rune, value string) {
	var format strings.Builder
	format.WriteByte('%')
	for _, flag := range []byte{'#', '0', '+', '-', ' '} {
		if s.Flag(int(flag)) {
			format.WriteByte(flag)
		}
	}
	if width, ok := s.Width(); ok {
		format.WriteString(strconv.Itoa(width))
	}
	if precision, ok := s.Precision(); ok {
		format.WriteByte('.')
		format.WriteString(strconv.Itoa(precision))
	}
	format.WriteRune(verb)
	_, _ = fmt.Fprintf(s, format.String(), value)
}

// FormatErrorStack returns err's message followed by the first stack found in
// the chain.
func FormatErrorStack(err error) string {
	if err == nil {
		return ""
	}
	var b strings.Builder
	b.WriteString(err.Error())
	if frames := firstStackFrames(err); len(frames) > 0 {
		for _, frame := range frames {
			_, _ = fmt.Fprintf(&b, "\n%s.%s\n\t%s:%d", frame.Package, frame.Name, frame.File, frame.LineNumber)
		}
	}
	return b.String()
}

func firstStackFrames(err error) []goerrors.StackFrame {
	for err != nil {
		if source, ok := err.(interface {
			StackFrames() []goerrors.StackFrame
		}); ok {
			if frames := source.StackFrames(); len(frames) > 0 {
				return frames
			}
		}
		switch e := err.(type) {
		case interface{ Unwrap() error }:
			err = e.Unwrap()
		case interface{ Unwrap() []error }:
			for _, child := range e.Unwrap() {
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
