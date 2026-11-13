package example

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/instacart/go-merrors/merrors"
)

// CustomError is a plain struct embedding BaseError. It does not implement
// Error(); BaseError owns message composition and formatting.
type CustomError struct {
	merrors.BaseError
}

func NewCustomError(opts ...merrors.Option) *CustomError {
	return &CustomError{BaseError: merrors.NewBaseError(opts...)}
}

// ExampleCustomError shows wrapped custom messages. Last message option wins,
// so constructors can provide defaults and callers can override them.
func ExampleCustomError() {
	original := merrors.New("original")

	err1 := NewCustomError(merrors.Wrapping(original), merrors.WithMessage("err1"))
	err2 := NewCustomError(merrors.Wrapping(err1), merrors.WithMessage("err2"))

	fmt.Println(err2.Error())
	// Output:
	// original
	// err1
	// err2
}

// CustomErrorWithSource formats its field into the message at construction.
type CustomErrorWithSource struct {
	merrors.BaseError
	source string
}

func (e *CustomErrorWithSource) Source() string { return e.source }

func NewCustomErrorWithSource(source, message string) *CustomErrorWithSource {
	return &CustomErrorWithSource{
		BaseError: merrors.NewBaseError(
			merrors.WithMessagef("%s (source = %s)", message, source),
		),
		source: source,
	}
}

func ExampleCustomErrorWithSource() {
	err := NewCustomErrorWithSource("source1", "err1")
	fmt.Println(err.Error())
	// Output: err1 (source = source1)
}

func TestNilWrapping(t *testing.T) {
	require.Nil(t, merrors.Wrap(nil))
	require.Nil(t, merrors.Wrap(nil, merrors.WithMessage("message")))
}
