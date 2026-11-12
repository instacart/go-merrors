// Every file in this directory is a test. The package has no non-test Go
// files, so nothing here is importable, nothing here is part of merrors' API,
// and `go test ./...` compiles and runs all of it.
package example

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/instacart/go-merrors/merrors"
)

// Example creates the smallest error this package can build.
//
// `%+v` prints the stacktrace as well, which is what you want while debugging
// and not in production: the output is not stable, so it is not asserted here.
// See [merrors.FormatErrorStack] and the example below for a log line instead.
func Example() {
	err := merrors.NewErrorBuilder().SetMessage("message").Build()

	fmt.Println(err.Error())
	// Output: message
}

// ExampleNewErrorBuilder wraps an error, adding the context of where it came
// from. SetMessage on a wrapping builder appends, so the chain reads
// innermost-first.
func ExampleNewErrorBuilder() {
	inner := merrors.NewErrorBuilder().SetMessage("innerError").Build()

	err := merrors.NewErrorBuilder().Wrap(inner).SetMessage("error calling methodA").Build()

	fmt.Println(err.Error())
	// Output:
	// innerError
	// error calling methodA
}

// TestFormatErrorStack is the "log it to Datadog" recipe: one call produces the
// message and the stacktrace that follows it, so the logger needs no knowledge
// of this package's types.
func TestFormatErrorStack(t *testing.T) {
	err := merrors.NewErrorBuilder().SetMessage("message").Build()

	formatted := merrors.FormatErrorStack(err)

	require.Contains(t, formatted, "message")
	require.Contains(t, formatted, "example.TestFormatErrorStack")
}
