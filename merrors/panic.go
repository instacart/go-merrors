package merrors

import "github.com/go-errors/errors"

// panicError is a recovered panic, as an error with the stack it was recovered at.
//
// It follows the convention BaseError documents for custom error types: the struct embeds
// the base error, the builder embeds the base builder, and the constructor goes through
// InitBaseErrorBuilder. Embedding is what supplies the stack, and the trim in StackFrames
// below is the only thing this type adds to it — so a panicError formats through %+v and
// FormatForLogger like any other error in this package, with nothing added to either.
type panicError struct {
	BaseError[*panicError]
}

// panicErrorBuilder builds a panicError.
type panicErrorBuilder struct {
	BaseErrorBuilder[*panicErrorBuilder, *panicError]
}

// newPanicErrorBuilder returns a builder for a panicError.
func newPanicErrorBuilder() *panicErrorBuilder {
	return InitBaseErrorBuilder(&panicErrorBuilder{}, func() *panicError { return &panicError{} })
}

// StackFrames implements hasStackFrames, with the runtime machinery trimmed off the front.
//
// It has to be trimmed by name rather than skipped by count: an explicit panic unwinds
// straight through runtime.gopanic, while a nil dereference goes through runtime.panicmem and
// runtime.sigpanic first. A skip that lands on the function that panicked for one lands two
// frames early for the other, and both look plausible. What is left is the panicking function
// first, which is where the runtime's own panic trace starts.
func (e *panicError) StackFrames() []errors.StackFrame {
	frames := e.BaseError.StackFrames()

	first := 0
	for first < len(frames) && frames[first].Package == "runtime" {
		first++
	}

	return frames[first:]
}

// RecoverAsError can be invoked in a deferred statement to automatically assign a recovered
// panic, if any, to the given non-nil error pointer. If there's no panic to recover, the
// given error pointer will preserve its previous value, as opposed to forcing it to become
// nil.
//
// It has to be deferred directly:
//
//	defer errors.RecoverAsError(&err)   // recovers
//
// and not through a closure:
//
//	defer func() { errors.RecoverAsError(&err) }()   // does not
//
// because recover only sees a panic when the function that deferred it calls recover
// itself. The second form lets the panic through and the process dies.
//
// The recovered value is carried in the message, and unwrapped to when it is itself an
// error, so errors.Is and errors.As reach it. It is not attached with Wrap: that takes the
// stack from the wrapped error whenever that error carries frames of its own — see
// BaseErrorBuilder.Wrap and CaptureStack — which for a panicked error means reporting where
// it was created instead of where it was raised.
//
// So the stack reported here is the panic site, and a cause that has one of its own keeps it:
// unwrapping reaches it, and it is what FormatErrorStack would print if this error had none.
// Only one of the two can be in the report — FormatErrorStack stops at the first non-empty
// frames in the chain — and the panic site is the one that cannot be recovered afterwards,
// because it is only capturable from here.
func RecoverAsError(err *error) {
	rec := recover()
	if rec == nil {
		return
	}

	builder := newPanicErrorBuilder().SetMessagef("panic (recovered): %v", rec)

	// A recovered value that is itself an error stays reachable with errors.Is and errors.As,
	// by unwrapping to it. This is set directly rather than through Wrap because Wrap takes
	// the stack from the wrapped error whenever it carries frames of its own — which for a
	// panicked error means reporting where it was created rather than where it was raised.
	// Setting it here leaves errForStack alone, so the stack is still the one below.
	if recoveredErr, ok := rec.(error); ok {
		builder.wrappedErr = recoveredErr
	}

	// Built here, in the deferred function, because the stack is captured as the error is
	// built: this is the last moment the frames that panicked are still live.
	//
	// CaptureStack is called with the recovery frames skipped, so the report starts at the
	// panic machinery and the function that panicked rather than at this one. Build calls
	// CaptureStack(1) itself, which then finds the stack already taken and leaves it alone.
	*err = builder.CaptureStack(recoverySkip).Build()
}

// recoverySkip is how many frames of this package sit between the panic and the frames worth
// reporting. CaptureStack adds one for itself and one for errors.Wrap, so this counts
// RecoverAsError and the builder call around it — plus one more, which drops the panic
// machinery. Where exactly that lands depends on the panic, and StackFrames trims the rest by
// name; either way the first frame reported is the function that panicked.
const recoverySkip = 2
