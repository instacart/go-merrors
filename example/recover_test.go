package example

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/instacart/go-merrors/merrors"
)

// ExampleRecoverAsError turns a panic into an error, so it is logged with a
// stacktrace like everything else.
//
// Defer it directly — `defer merrors.RecoverAsError(&err)` — and not through a
// closure: recover only sees a panic when the function that deferred it calls
// recover itself, so `defer func() { merrors.RecoverAsError(&err) }()` lets the
// panic through and the process dies.
func ExampleRecoverAsError() {
	work := func() (err error) {
		defer merrors.RecoverAsError(&err)

		panic("boom")
	}

	fmt.Println(work().Error())
	// Output: panic (recovered): boom
}

// TestRecoverAsErrorKeepsExisting pins the no-panic case: the error pointer
// keeps whatever it already held, so it is safe to defer on a path that may not
// panic at all.
func TestRecoverAsErrorKeepsExisting(t *testing.T) {
	existing := merrors.NewErrorBuilder().SetMessage("existing").Build()

	work := func() (err error) {
		err = existing
		defer merrors.RecoverAsError(&err)

		return err
	}

	require.Equal(t, "existing", work().Error())
}
