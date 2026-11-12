package merrors

import (
	stderrors "errors"
	"fmt"
	"regexp"
	"testing"

	goerr "github.com/go-errors/errors"
	"github.com/stretchr/testify/assert"
)

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
		assert.Equal(t, fmt.Errorf("bla"), Unwrap(err))

		err = Wrap(source).(*errorImpl)
		assert.Equal(t, "bla", err.Error())
		// For go errors we just copy the stack frames from the wrapped error.
		assert.Greater(t, len(err.StackFrames()), 0)
		assert.Equal(t, fmt.Errorf("bla"), Unwrap(err))

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

	t.Run("As", func(t *testing.T) {
		original := New("thecause")
		var target Error
		assert.True(t, As(original, &target))
		assert.Equal(t, original, target)
	})
}
