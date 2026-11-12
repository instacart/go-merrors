package example

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/instacart/go-merrors/merrors"
)

// CustomError is the smallest useful custom error.
//
// It embeds BaseError, so wrapping, stacktraces, cloning and formatting are
// inherited, and all that is left to write is a builder. Callers get a typed
// error and still pass it around as a merrors.Error.
type CustomError struct {
	merrors.BaseError[*CustomError]
}

type CustomErrorBuilder struct {
	merrors.BaseErrorBuilder[*CustomErrorBuilder, *CustomError]
}

func NewCustomErrorBuilder() *CustomErrorBuilder {
	return merrors.InitBaseErrorBuilder(&CustomErrorBuilder{}, func() *CustomError { return &CustomError{} })
}

// ExampleInitBaseErrorBuilder shows the minimal custom error in use: it wraps,
// and each layer's message appends to the chain.
func ExampleInitBaseErrorBuilder() {
	original := merrors.NewErrorBuilder().SetMessage("original").Build()

	err1 := NewCustomErrorBuilder().SetMessage("err1").Wrap(original).Build()
	err2 := NewCustomErrorBuilder().SetMessage("err2").Wrap(err1).Build()

	fmt.Println(err2.Error())
	// Output:
	// original
	// err1
	// err2
}

// TestCustomErrorWrapNil pins a behaviour that is easy to get wrong: wrapping
// nil returns a nil error, not a non-nil interface holding a nil pointer.
//
// The second assertion is the point. require.Nil passes for both, and the bug
// this guards against — a boxed nil that equals nil to a viewer but not to `==` —
// is only caught by comparing the interface to nil directly.
func TestCustomErrorWrapNil(t *testing.T) {
	err := NewCustomErrorBuilder().SetMessage("message").Wrap(nil).Build()

	require.Nil(t, err)
	require.True(t, err == nil)
}

// CustomErrorWithSource adds a field and overrides how the error prints.
//
// Two hooks make this work: PopulateBuilderResult copies state from the builder
// onto the built error, and PopulateClone copies it again whenever the error is
// wrapped or cloned — without the second one the field would be lost on every
// wrap.
type CustomErrorWithSource struct {
	merrors.BaseError[*CustomErrorWithSource]

	source string
}

func (c *CustomErrorWithSource) Error() string {
	return "CustomError: " + c.BaseError.Error() + " :/CustomError"
}

func (c *CustomErrorWithSource) Source() string {
	return c.source
}

func (c *CustomErrorWithSource) PopulateClone(clone *CustomErrorWithSource) {
	c.BaseError.PopulateClone(clone)
	clone.source = c.source
}

type CustomErrorWithSourceBuilder struct {
	merrors.BaseErrorBuilder[*CustomErrorWithSourceBuilder, *CustomErrorWithSource]

	source string
}

func (b *CustomErrorWithSourceBuilder) SetSource(source string) *CustomErrorWithSourceBuilder {
	b.source = source
	return b
}

func (b *CustomErrorWithSourceBuilder) PopulateBuilderResult(result *CustomErrorWithSource) {
	b.SetMessage(b.GetMessage() + " (source = " + b.source + ")")
	result.source = b.source
	b.BaseErrorBuilder.PopulateBuilderResult(result)
}

func NewCustomErrorWithSourceBuilder() *CustomErrorWithSourceBuilder {
	return merrors.InitBaseErrorBuilder(&CustomErrorWithSourceBuilder{}, func() *CustomErrorWithSource { return &CustomErrorWithSource{} })
}

// ExampleInitBaseErrorBuilder_withProperties shows a custom error carrying a
// field and changing its own message.
func ExampleInitBaseErrorBuilder_withProperties() {
	original := merrors.NewErrorBuilder().SetMessage("original").Build()

	err := NewCustomErrorWithSourceBuilder().
		Wrap(original).
		SetSource("source1").
		SetMessage("err1").
		Build()

	fmt.Println(err.Error())
	// Output:
	// CustomError: original
	// err1 (source = source1) :/CustomError
}
