package merrors

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRecoverAsError(t *testing.T) {
	var err error
	func() {
		defer RecoverAsError(&err)
		panic("boom")
	}()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "panic (recovered): boom")
	stacked, ok := err.(Error)
	require.True(t, ok)
	assert.NotEmpty(t, stacked.StackFrames())
}

func panicOneCallDown() { panic("nested panic") }

func recursePanic(depth int) {
	if depth == 0 {
		panic("deep panic")
	}
	recursePanic(depth - 1)
}

func TestRecoverAsErrorDeepStack(t *testing.T) {
	var err error
	func() {
		defer RecoverAsError(&err)
		recursePanic(512)
	}()

	stacked, ok := err.(Error)
	require.True(t, ok)
	assert.Greater(t, len(stacked.StackFrames()), 64)
}

func TestRecoverAsErrorUsesPanicSite(t *testing.T) {
	var err error
	func() {
		defer RecoverAsError(&err)
		panicOneCallDown()
	}()

	stacked, ok := err.(Error)
	require.True(t, ok)
	frames := stacked.StackFrames()
	require.NotEmpty(t, frames)
	assert.Contains(t, fmt.Sprintf("%s.%s", frames[0].Package, frames[0].Name), "panicOneCallDown")
}

func TestRecoverAsErrorKeepsPanickedIdentityWithoutCreationStack(t *testing.T) {
	original := New("created earlier")
	var err error
	func() {
		defer RecoverAsError(&err)
		panic(original)
	}()

	require.ErrorIs(t, err, original)
	stacked, ok := err.(Error)
	require.True(t, ok)
	assert.NotEqual(t, original.(Error).StackFrames(), stacked.StackFrames())
}
