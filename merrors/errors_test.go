package merrors

import (
	stderrors "errors"
	"fmt"
	"regexp"
	"sync"
	"testing"
	"time"

	goerr "github.com/go-errors/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type stackFrameError struct {
	frames []goerr.StackFrame
}

func (e stackFrameError) Error() string { return "stack frame error" }

func (e stackFrameError) StackFrames() []goerr.StackFrame { return e.frames }

type reentrantStackError struct {
	cause error
}

func (e reentrantStackError) Error() string { return "reentrant stack error" }

func (e reentrantStackError) StackFrames() []goerr.StackFrame {
	return WrapMessage(e.cause, "inner").StackFrames()
}

type customIsSentinel struct{}

func (s *customIsSentinel) Error() string { return "custom sentinel" }

type customIsError struct {
	BaseError[*customIsError]
}

func (e *customIsError) Is(target error) bool {
	_, ok := target.(*customIsSentinel)
	return ok
}

type customIsErrorBuilder struct {
	BaseErrorBuilder[*customIsErrorBuilder, *customIsError]
}

func newCustomIsErrorBuilder() *customIsErrorBuilder {
	return InitBaseErrorBuilder(&customIsErrorBuilder{}, func() *customIsError { return &customIsError{} })
}

func TestErrors(t *testing.T) {
	normalizeStack := func(stack string) string {
		stack = regexp.MustCompile("\n\t.*/errors_test.go:").ReplaceAllString(stack, "\n\t/root/errors_test.go:")
		stack = regexp.MustCompile("TestErrors.func[0-9]*").ReplaceAllString(stack, "TestErrors.funcN")
		return stack
	}

	t.Run("Default", func(t *testing.T) {
		err := NewErrorBuilder().Build()
		assert.Equal(t, "", err.Error())
		assert.Greater(t, len(err.StackFrames()), 0)
	})

	t.Run("Wrap", func(t *testing.T) {
		source := goerr.New("bla")
		err := NewErrorBuilder().Wrap(source).Build()
		assert.Equal(t, "bla", err.Error())
		// For go errors we just copy the stack frames from the wrapped error.
		assert.Greater(t, len(err.StackFrames()), 0)
		assert.Same(t, source, Unwrap(err))

		err = Wrap(source).(*errorImpl)
		assert.Equal(t, "bla", err.Error())
		// For go errors we just copy the stack frames from the wrapped error.
		assert.Greater(t, len(err.StackFrames()), 0)
		assert.Same(t, source, Unwrap(err))

		// Ensuring that an already wrapped error is not wrapped again.
		assert.Equal(t, err, Wrap(err))

		err2 := NewErrorBuilder().Wrap(err).SetMessage("message").Build()
		assert.Equal(t, "bla\nmessage", err2.Error())
		assert.Equal(t, "bla\nmessage", WrapMessage(err, "message").Error())
		// For other error types we don't copy the stack frames from the
		// wrapped error and just leave it empty as sentry can get it from
		// the wrapped error.
		assert.Len(t, stderrors.Unwrap(err2).(Error).StackFrames(), 0)
		assert.Equal(t, err.StackFrames(), err2.StackFrames())
		assert.Equal(t, err.Error(), stderrors.Unwrap(err2).Error())
	})

	t.Run("WrapNil", func(t *testing.T) {
		assert.Nil(t, NewErrorBuilder().Wrap(nil).Build())
		// A nil implementation boxed into the interface would satisfy assert.Nil but not == nil.
		assert.True(t, NewErrorBuilder().Wrap(nil).Build() == nil)
		assert.Nil(t, Wrap(nil))
		assert.Nil(t, WrapMessage(nil, "message"))
		assert.Nil(t, WrapMessagef(nil, "message"))
		assert.Nil(t, NewErrorBuilder().Wrap(nil).SetMessage("message").Build())
	})

	t.Run("Format", func(t *testing.T) {
		err := NewErrorBuilder().SetMessage("message").Build()

		assert.Equal(t,
			normalizeStack(fmt.Sprintf("%v", err)),
			"message",
		)

		fmt.Println(normalizeStack(fmt.Sprintf("%+v", err)))
		assert.Contains(t,
			normalizeStack(fmt.Sprintf("%+v", err)),
			"message\n"+
				"github.com/instacart/go-merrors/merrors.TestErrors.funcN\n"+
				"\t/root/errors_test.go:",
		)

		assert.Equal(t, `"message"`, fmt.Sprintf("%q", err))
		assert.Equal(t, "6d657373616765", fmt.Sprintf("%x", err))
		assert.Equal(t, "6D657373616765", fmt.Sprintf("%X", err))
	})

	t.Run("FormatErrorStack", func(t *testing.T) {
		var err error
		err = NewErrorBuilder().SetMessage("message").Build()

		assert.Contains(t,
			normalizeStack(FormatErrorStack(err)),
			"message\n"+
				"github.com/instacart/go-merrors/merrors.TestErrors.funcN\n"+
				"\t/root/errors_test.go:",
		)

		err = NewErrorBuilder().SetMessage("outer").Wrap(NewErrorBuilder().SetMessage("inner").Build()).Build()

		assert.Contains(t,
			normalizeStack(FormatErrorStack(err)),
			"inner\n"+
				"outer\n"+
				"github.com/instacart/go-merrors/merrors.TestErrors.funcN\n"+
				"\t/root/errors_test.go:",
		)

		err = Errorf("message %d", 123)

		assert.Contains(t,
			normalizeStack(FormatErrorStack(err)),
			"message 123\n"+
				"github.com/instacart/go-merrors/merrors.TestErrors.funcN\n"+
				"\t/root/errors_test.go:",
		)

		err = New("new message")

		assert.Contains(t,
			normalizeStack(FormatErrorStack(err)),
			"new message\n"+
				"github.com/instacart/go-merrors/merrors.TestErrors.funcN\n"+
				"\t/root/errors_test.go:",
		)

		err = Wrap(stderrors.New("std"))

		assert.Contains(t,
			normalizeStack(FormatErrorStack(err)),
			"std\n"+
				"github.com/instacart/go-merrors/merrors.TestErrors.funcN\n"+
				"\t/root/errors_test.go:",
		)

		err = WrapMessage(stderrors.New("std"), "msg")

		assert.Contains(t,
			normalizeStack(FormatErrorStack(err)),
			"std\n"+
				"msg\n"+
				"github.com/instacart/go-merrors/merrors.TestErrors.funcN\n"+
				"\t/root/errors_test.go:",
		)

		err = WrapMessagef(stderrors.New("std"), "a: %d", 2)

		assert.Contains(t,
			normalizeStack(FormatErrorStack(err)),
			"std\n"+
				"a: 2\n"+
				"github.com/instacart/go-merrors/merrors.TestErrors.funcN\n"+
				"\t/root/errors_test.go:",
		)

		err = stderrors.New("std")
		assert.Equal(t, FormatErrorStack(err), "std")
	})

	t.Run("FormatErrorStackMultiError", func(t *testing.T) {
		// The first child has no stack; the stack must be found in the second.
		joined := stderrors.Join(stderrors.New("other"), NewErrorBuilder().SetMessage("merrors").Build())
		formatted := normalizeStack(FormatErrorStack(joined))
		assert.Contains(t, formatted, "merrors")
		assert.Contains(t, formatted,
			"github.com/instacart/go-merrors/merrors.TestErrors.funcN\n"+
				"\t/root/errors_test.go:",
		)

		// Multiple %w verbs produce an Unwrap() []error as well.
		multiWrapped := fmt.Errorf("%w: %w", stderrors.New("first"), NewErrorBuilder().SetMessage("merrors").Build())
		formatted = normalizeStack(FormatErrorStack(multiWrapped))
		assert.Contains(t, formatted, "merrors")
		assert.Contains(t, formatted,
			"github.com/instacart/go-merrors/merrors.TestErrors.funcN\n"+
				"\t/root/errors_test.go:",
		)

		// A nested multi error is searched depth-first.
		nested := fmt.Errorf("outer: %w", stderrors.Join(stderrors.New("other"), Errorf("nested merrors")))
		formatted = normalizeStack(FormatErrorStack(nested))
		assert.Contains(t, formatted, "nested merrors")
		assert.Contains(t, formatted,
			"github.com/instacart/go-merrors/merrors.TestErrors.funcN\n"+
				"\t/root/errors_test.go:",
		)
	})

	t.Run("WrapWithoutStack", func(t *testing.T) {
		original := NewErrorBuilder().SetMessage("original").Build()
		assert.Len(t, original.WithoutStack().StackFrames(), 0)

		err := NewErrorBuilder().Wrap(original.WithoutStack()).Build()
		assert.Equal(t, "original", err.Error())
		// A stack-less wrapped error must not suppress the captured stack.
		assert.Greater(t, len(err.StackFrames()), 0)

		err = WrapMessage(original.WithoutStack(), "message")
		assert.Equal(t, "original\nmessage", err.Error())
		assert.Greater(t, len(err.StackFrames()), 0)
	})

	t.Run("SetMessage", func(t *testing.T) {
		err := NewErrorBuilder().SetMessage("message").Build()
		assert.Equal(t, "message", err.Error())

		err = NewErrorBuilder().Wrap(goerr.New("bla")).SetMessage("message").Build()
		assert.Equal(t, "bla\nmessage", err.Error())

		err = NewErrorBuilder().SetMessage("message").Wrap(goerr.New("bla")).Build()
		assert.Equal(t, "bla\nmessage", err.Error())
	})

	t.Run("SetMessagef", func(t *testing.T) {
		err := NewErrorBuilder().SetMessagef("message %d", 123).Build()
		assert.Equal(t, "message 123", err.Error())
	})

	t.Run("Clone", func(t *testing.T) {
		err := NewErrorBuilder().SetMessage("message").Build()
		clone := err.Clone()
		assert.False(t, err == clone)
		assert.Equal(t, err.Error(), clone.Error())
		assert.Equal(t, err.StackFrames(), clone.StackFrames())
	})

	t.Run("WithAppendMessage", func(t *testing.T) {
		err := NewErrorBuilder().SetMessage("message").Build()
		err2 := err.WithAppendMessage("message2")
		// Initial error should not be modified
		assert.Equal(t, "message", err.Error())
		// New error should have the new message
		assert.Equal(t, "message\nmessage2", err2.Error())
	})

	t.Run("WithAppendMessagef", func(t *testing.T) {
		err := NewErrorBuilder().SetMessage("message").Build()
		err2 := err.WithAppendMessagef("message2 %d", 123)
		// Initial error should not be modified
		assert.Equal(t, "message", err.Error())
		// New error should have the new message
		assert.Equal(t, "message\nmessage2 123", err2.Error())
	})

	t.Run("WithMessageOverride", func(t *testing.T) {
		err := NewErrorBuilder().SetMessage("message").Build()
		err2 := err.WithMessageOverride("new message")
		assert.Equal(t, "new message", err2.Error())
	})

	t.Run("genericBaseError", func(t *testing.T) {
		err := NewErrorBuilder().SetMessage("message").Build()
		assert.Equal(t, err.genericBaseError(), err.genericBaseError())
	})

	t.Run("Errorf", func(t *testing.T) {
		cause := stderrors.New("thecause")
		err := Errorf("error with cause = %w", cause)
		assert.Equal(t, "error with cause = thecause", err.Error())
		assert.Equal(t, cause, Unwrap(err))
	})

	t.Run("ErrorfMultipleCauses", func(t *testing.T) {
		first := stderrors.New("first")
		second := stderrors.New("second")
		err := Errorf("both: %w and %w", first, second)
		assert.Equal(t, "both: first and second", err.Error())
		assert.True(t, stderrors.Is(err, first))
		assert.True(t, stderrors.Is(err, second))
	})

	t.Run("WrapPreservesErrorIdentity", func(t *testing.T) {
		cause := New("cause")
		assert.True(t, stderrors.Is(WrapMessage(cause, "context"), cause))
		assert.True(t, stderrors.Is(Errorf("context: %w", cause), cause))
	})

	t.Run("WrapStacklessErrorCapturesStack", func(t *testing.T) {
		original := NewErrorBuilder().SetMessage("cause").Build()
		stackless := original.WithoutStack()

		wrapped := Wrap(stackless)
		assert.NotEmpty(t, wrapped.StackFrames())
		assert.True(t, stderrors.Is(wrapped, stackless))
	})

	t.Run("WrapStackedErrorReturnsSame", func(t *testing.T) {
		original := New("cause")
		assert.Same(t, original, Wrap(original))
	})

	t.Run("ErrorfPreservesGoErrorsOperand", func(t *testing.T) {
		cause := goerr.New("cause")
		err := Errorf("context: %w", cause)

		assert.True(t, stderrors.Is(err, cause))
		var target *goerr.Error
		assert.True(t, stderrors.As(err, &target))
		assert.Same(t, cause, target)
	})

	t.Run("WrapPreservesIdentityWithCustomIs", func(t *testing.T) {
		original := newCustomIsErrorBuilder().SetMessage("custom").CaptureStack(1).Build()
		sentinel := &customIsSentinel{}

		assert.True(t, stderrors.Is(original, sentinel))
		assert.True(t, stderrors.Is(original, original))

		wrapped := WrapMessage(original, "context")
		assert.True(t, stderrors.Is(wrapped, original))
		assert.True(t, stderrors.Is(wrapped, sentinel))

		var target *customIsError
		assert.True(t, stderrors.As(wrapped, &target))

		formatted := Errorf("context: %w", original)
		assert.True(t, stderrors.Is(formatted, original))
		assert.True(t, stderrors.Is(formatted, sentinel))
	})

	t.Run("WrapAfterWithoutStackPreservesIdentity", func(t *testing.T) {
		original := NewErrorBuilder().SetMessage("original").Build()
		stackless := original.WithoutStack()
		wrapped := WrapMessage(stackless, "context")

		assert.True(t, stderrors.Is(wrapped, stackless))
		assert.True(t, stderrors.Is(wrapped, original))
	})

	t.Run("WrapReplacesStack", func(t *testing.T) {
		stacked := stackFrameError{frames: []goerr.StackFrame{{Package: "sentinel-package"}}}
		err := NewErrorBuilder().Wrap(stacked).Wrap(stderrors.New("plain")).Build()

		for _, frame := range err.StackFrames() {
			assert.NotEqual(t, "sentinel-package", frame.Package)
		}
	})

	t.Run("ConcurrentStackFrames", func(t *testing.T) {
		// Frames are materialized lazily, so every goroutine below races to be
		// the first to ask for them. Clones share the stack provider, so the
		// lookup happens once, under the provider's Once.
		const goroutines = 64

		original := NewErrorBuilder().SetMessage("concurrent").Build()
		errs := make([]Error, goroutines)
		for i := range errs {
			if i == 0 {
				errs[i] = original
				continue
			}
			errs[i] = original.WithAppendMessage("clone")
		}

		start := make(chan struct{})
		var wg sync.WaitGroup
		wg.Add(goroutines)
		for _, err := range errs {
			go func(err Error) {
				defer wg.Done()
				<-start
				assert.NotEmpty(t, err.StackFrames())
				assert.NotEmpty(t, FormatErrorStack(err))
				assert.NotEmpty(t, fmt.Sprintf("%+v", err))
			}(err)
		}
		close(start)
		wg.Wait()

		for _, err := range errs {
			assert.Equal(t, errs[0].StackFrames(), err.StackFrames())
		}
	})

	t.Run("ConcurrentWrapExternalGoErrors", func(t *testing.T) {
		// The shared go-errors sentinel has an unsynchronized lazy frame cache.
		// Concurrent WrapMessage calls must not race its first StackFrames call.
		const goroutines = 64
		cause := goerr.New("shared sentinel")
		start := make(chan struct{})
		var wg sync.WaitGroup
		wg.Add(goroutines)
		for i := 0; i < goroutines; i++ {
			go func() {
				defer wg.Done()
				<-start
				err := WrapMessage(cause, "context")
				assert.NotEmpty(t, err.StackFrames())
				assert.NotEmpty(t, FormatErrorStack(err))
			}()
		}
		close(start)
		wg.Wait()
	})

	t.Run("WrappingIdentityErrorCapturesStack", func(t *testing.T) {
		// Unwrap(WrapMessage(...)) is an identityError, which is stack-less.
		// Wrapping it must not adopt an empty provider and suppress CaptureStack.
		cause := Unwrap(WrapMessage(New("cause"), "context"))
		require.NotNil(t, cause)

		stacked, ok := cause.(interface{ StackFrames() []goerr.StackFrame })
		require.True(t, ok)
		assert.Empty(t, stacked.StackFrames())

		wrapped := WrapMessage(cause, "retry")
		assert.NotEmpty(t, wrapped.StackFrames())
	})

	t.Run("ReentrantExternalStackFramesDoesNotDeadlock", func(t *testing.T) {
		// An external wrapper may implement StackFrames by wrapping its cause
		// in this package. The outer wrap must not hold a lock across that
		// callback, or the inner wrap deadlocks.
		done := make(chan Error, 1)
		go func() {
			done <- WrapMessage(reentrantStackError{cause: goerr.New("inner cause")}, "outer")
		}()

		select {
		case err := <-done:
			assert.NotEmpty(t, err.StackFrames())
		case <-time.After(2 * time.Second):
			t.Fatal("wrapping a reentrant external stack source deadlocked")
		}
	})

	t.Run("As", func(t *testing.T) {
		original := New("thecause")
		var target Error
		assert.True(t, As(original, &target))
		assert.Equal(t, original, target)
	})
}
