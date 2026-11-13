// Every file in this directory is a test, so this package is examples-only and
// cannot be imported.
package example

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/instacart/go-merrors/merrors"
)

// Example creates the smallest error.
func Example() {
	err := merrors.New("message")

	fmt.Println(err.Error())
	// Output: message
}

// ExampleWrap adds context. The wrapped cause stays first.
func ExampleWrap() {
	inner := merrors.New("innerError")
	err := merrors.Wrap(inner, merrors.WithMessage("error calling methodA"))

	fmt.Println(err.Error())
	// Output:
	// innerError
	// error calling methodA
}

// TestFormatErrorStack shows the log-line recipe.
func TestFormatErrorStack(t *testing.T) {
	err := merrors.New("message")
	formatted := merrors.FormatErrorStack(err)

	require.Contains(t, formatted, "message")
	require.Contains(t, formatted, "example.TestFormatErrorStack")
}
