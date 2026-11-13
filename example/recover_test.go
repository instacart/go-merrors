package example

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/instacart/go-merrors/merrors"
)

// ExampleRecoverAsError turns a panic into an error with a recovery stack.
func ExampleRecoverAsError() {
	work := func() (err error) {
		defer merrors.RecoverAsError(&err)
		panic("boom")
	}

	fmt.Println(work().Error())
	// Output: panic (recovered): boom
}

func TestRecoverAsErrorKeepsExisting(t *testing.T) {
	existing := merrors.New("existing")

	work := func() (err error) {
		err = existing
		defer merrors.RecoverAsError(&err)
		return err
	}

	require.Equal(t, "existing", work().Error())
}
