package merrors

import (
	stderrors "errors"
	"fmt"
	"strings"
	"testing"

	goerrors "github.com/go-errors/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type customError struct {
	BaseError
	Source string
}

func newCustomError(source string, opts ...Option) *customError {
	return &customError{BaseError: NewBaseError(opts...), Source: source}
}

func TestStackAtConstructorCaller(t *testing.T) {
	err := newCustomError("source", WithMessage("custom"))
	require.NotEmpty(t, err.StackFrames())
	assert.Contains(t, fmt.Sprintf("%s.%s", err.StackFrames()[0].Package, err.StackFrames()[0].Name), "TestStackAtConstructorCaller")
}

func newSkippedBase(opts ...Option) BaseError {
	return NewBaseError(opts...)
}

func helperCaller(opts ...Option) BaseError {
	return newSkippedBase(opts...)
}

func TestSkipFrames(t *testing.T) {
	base := helperCaller(WithMessage("base"), SkipFrames(1))
	require.NotEmpty(t, base.StackFrames())
	assert.Contains(t, fmt.Sprintf("%s.%s", base.StackFrames()[0].Package, base.StackFrames()[0].Name), "TestSkipFrames")
}

func TestWrappingReusesStack(t *testing.T) {
	inner := New("inner")
	outer := Wrap(inner, WithMessage("outer"))
	require.NotEmpty(t, outer.(Error).StackFrames())
	assert.Equal(t, inner.(Error).StackFrames(), outer.(Error).StackFrames())
	assert.Equal(t, "inner\nouter", outer.Error())
}

func TestWrapNoOptionsReusesExistingError(t *testing.T) {
	inner := New("inner")
	assert.Same(t, inner, Wrap(inner))
}

func TestWrappingPlainErrorCapturesStack(t *testing.T) {
	outer := Wrap(stderrors.New("plain"), WithMessage("outer"))
	assert.NotEmpty(t, outer.(Error).StackFrames())
	assert.Equal(t, "plain\nouter", outer.Error())
}

func TestWithoutStack(t *testing.T) {
	err := NewBaseError(WithMessage("sentinel"), WithoutStack())
	assert.Empty(t, (&err).StackFrames())
	assert.Equal(t, "sentinel", (&err).Error())
}

func TestDefaultMessageCombiner(t *testing.T) {
	tests := []struct {
		wrapped, current, want string
	}{
		{"", "", ""},
		{"wrapped", "", "wrapped"},
		{"", "current", "current"},
		{"wrapped", "current", "wrapped\ncurrent"},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.want, DefaultMessageCombiner(tt.wrapped, tt.current))
	}
}

func TestMessageCombinerLastWins(t *testing.T) {
	var wrapped, current string
	err := NewBaseError(
		Wrapping(stderrors.New("wrapped")),
		WithMessage("current"),
		WithMessageCombiner(func(w, c string) string { return "first" }),
		WithMessageCombiner(func(w, c string) string {
			wrapped, current = w, c
			return c + " | " + w
		}),
	)

	assert.Equal(t, "wrapped", wrapped)
	assert.Equal(t, "current", current)
	assert.Equal(t, "current | wrapped", (&err).Error())
}

func TestLastMessageWins(t *testing.T) {
	err := NewBaseError(WithMessage("default"), WithMessagef("caller %d", 2))
	assert.Equal(t, "caller 2", (&err).Error())
}

func TestEmptyMessageDoesNotAddBlankLine(t *testing.T) {
	inner := New("inner")
	outer := newCustomError("source", Wrapping(inner))
	assert.Equal(t, "inner", outer.Error())
}

func TestOmitWrappedMessage(t *testing.T) {
	err := NewBaseError(Wrapping(stderrors.New("cause")), WithMessage("outer"), OmitWrappedMessage())
	assert.Equal(t, "outer", (&err).Error())
	assert.ErrorIs(t, &err, (&err).Unwrap())
}

func TestErrorsIsAndAsThroughCustomAndJoin(t *testing.T) {
	inner := New("inner")
	custom := newCustomError("source", Wrapping(inner), WithMessage("custom"))
	assert.True(t, stderrors.Is(custom, inner))

	wrapped := fmt.Errorf("context: %w", inner)
	assert.True(t, stderrors.Is(Wrap(wrapped, WithMessage("outer")), inner))
	assert.True(t, stderrors.Is(stderrors.Join(inner, stderrors.New("other")), inner))
}

func TestFormattingUsesBaseError(t *testing.T) {
	custom := newCustomError("source", WithMessage("custom"))
	require.NotEmpty(t, custom.StackFrames())

	assert.Equal(t, "custom", fmt.Sprintf("%v", custom))
	assert.Contains(t, fmt.Sprintf("%+v", custom), "custom")
	assert.Contains(t, FormatErrorStack(custom), "TestFormattingUsesBaseError")
}

type overridesError struct {
	BaseError
}

func (e *overridesError) Error() string { return "overridden" }

type overridesStack struct {
	BaseError
}

func (e *overridesStack) StackFrames() []goerrors.StackFrame { return nil }

type overridesFormat struct {
	BaseError
}

func (e *overridesFormat) Format(s fmt.State, verb rune) { _, _ = fmt.Fprint(s, "overridden") }

func TestCheckErrorType(t *testing.T) {
	assert.NoError(t, CheckErrorType(newCustomError("source", WithMessage("message"))))
	assert.Error(t, CheckErrorType(&overridesError{BaseError: NewBaseError(WithMessage("message"))}))
	assert.Error(t, CheckErrorType(&overridesStack{BaseError: NewBaseError(WithMessage("message"))}))
	assert.Error(t, CheckErrorType(&overridesFormat{BaseError: NewBaseError(WithMessage("message"))}))
	assert.Error(t, CheckErrorType(nil))
}

func TestWrappingSharesStackProvider(t *testing.T) {
	inner := New("inner")
	outer := Wrap(inner, WithMessage("outer"))
	require.Same(t, inner.(Error).merrorsBase().stack, outer.(Error).merrorsBase().stack)
}

func TestEmptyMerrorsStackCapturesReplacement(t *testing.T) {
	inner := newCustomError("source", WithMessage("inner"), SkipFrames(1000))
	require.Empty(t, inner.StackFrames())

	for name, outer := range map[string]error{
		"Wrap":          Wrap(inner, WithMessage("outer")),
		"Errorf":        Errorf("outer: %w", inner),
		"WrapNoOptions": Wrap(inner),
	} {
		t.Run(name, func(t *testing.T) {
			require.NotSame(t, inner, outer)
			frames := outer.(Error).StackFrames()
			require.NotEmpty(t, frames)
			assert.Contains(t, fmt.Sprintf("%s.%s", frames[0].Package, frames[0].Name), "TestEmptyMerrorsStackCapturesReplacement")
		})
	}
}

func TestWrappingHonorsExplicitStackFrames(t *testing.T) {
	frames := []goerrors.StackFrame{{
		Package:    "example",
		Name:       "explicit",
		File:       "example.go",
		LineNumber: 1,
	}}
	inner := newCustomError("source", WithMessage("inner"), WithStackFrames(frames))
	outer := Wrap(inner, WithMessage("outer"))
	assert.Equal(t, frames, outer.(Error).StackFrames())
}

func TestWrappingThroughFmtErrorKeepsStackAndIdentity(t *testing.T) {
	inner := newCustomError("custom", WithMessage("inner"))
	wrapped := fmt.Errorf("ctx: %w", inner)
	outer := Wrap(wrapped, WithMessage("outer"))

	assert.Equal(t, "ctx: inner\nouter", outer.Error())
	assert.True(t, stderrors.Is(outer, inner))

	var found *customError
	require.True(t, stderrors.As(outer, &found))
	assert.Same(t, inner, found)
	assert.Same(t, inner.BaseError.stack, outer.(Error).merrorsBase().stack)
}

func TestFormattingAgreesOnMessage(t *testing.T) {
	custom := newCustomError("source", WithMessage("message"))
	formatted := []string{
		custom.Error(),
		fmt.Sprintf("%v", custom),
		fmt.Sprintf("%s", custom),
		fmt.Sprintf("%+v", custom),
		FormatErrorStack(custom),
	}
	for _, got := range formatted {
		assert.Equal(t, "message", strings.SplitN(got, "\n", 2)[0], "unexpected formatting: %q", got)
	}
}

func TestNilWrapping(t *testing.T) {
	assert.Nil(t, Wrap(nil))
	assert.Nil(t, Wrap(nil, WithMessage("message")))
	assert.Nil(t, Wrap(nil, WithMessagef("message %d", 1)))
}

func TestNewStackPointsAtCaller(t *testing.T) {
	err := New("message")
	frames := err.(Error).StackFrames()
	require.NotEmpty(t, frames)
	assert.Contains(t, fmt.Sprintf("%s.%s", frames[0].Package, frames[0].Name), "TestNewStackPointsAtCaller")
}

func TestExternalStackReuse(t *testing.T) {
	external := goerrors.New("external")
	outer := Wrap(external, WithMessage("outer"))
	assert.Equal(t, external.StackFrames(), outer.(Error).StackFrames())
}

func TestFormattingPreservesWidthAndPrecision(t *testing.T) {
	err := New("abcdef")
	assert.Equal(t, "abc", fmt.Sprintf("%.3s", err))
	assert.Equal(t, "    abcdef", fmt.Sprintf("%10s", err))
	assert.Equal(t, `"abcdef"`, fmt.Sprintf("%q", err))
}

type emptyStackError struct{}

func (emptyStackError) Error() string                      { return "empty external" }
func (emptyStackError) StackFrames() []goerrors.StackFrame { return nil }

func TestStacklessExternalSourceCapturesReplacement(t *testing.T) {
	outer := Wrap(emptyStackError{}, WithMessage("outer"))
	assert.NotEmpty(t, outer.(Error).StackFrames())
}

func TestJoinSkipsEmptyExternalAndFindsLaterStack(t *testing.T) {
	cause := goerrors.New("cause")
	joined := stderrors.Join(emptyStackError{}, cause)
	outer := Wrap(joined, WithMessage("outer"))
	assert.Equal(t, cause.StackFrames(), outer.(Error).StackFrames())
}

func TestConcurrentExternalStackReuse(t *testing.T) {
	cause := goerrors.New("cause")
	const goroutines = 32
	start := make(chan struct{})
	done := make(chan struct{}, goroutines)
	for i := 0; i < goroutines; i++ {
		go func() {
			<-start
			assert.NotEmpty(t, Wrap(cause, WithMessage("outer")).(Error).StackFrames())
			done <- struct{}{}
		}()
	}
	close(start)
	for i := 0; i < goroutines; i++ {
		<-done
	}
}
