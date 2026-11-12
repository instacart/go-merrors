package merrors

import (
	stderrors "errors"
	"fmt"
	"os"
	"os/exec"
	"reflect"
	"runtime"
	"testing"

	goerr "github.com/go-errors/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The panic helpers below must not be inlined: a stack frame names the outermost
// function for an inlined PC, so the panic site would be reported as its caller under
// some build modes (the race detector's, for one) and the assertions below would have
// nothing to match.

// panicOneCallDown panics one frame below whoever called it, so a test can tell the frame
// that panicked from the frame that recovered it.
//
//go:noinline
func panicOneCallDown() {
	panic("nested panic")
}

// nilDereference panics the way a nil pointer does, which is not the same stack as an
// explicit panic: it unwinds through runtime.panicmem and runtime.sigpanic first.
//
//go:noinline
func nilDereference() {
	var p *int
	_ = *p
}

// panicWith raises a panic with the value it is given, so a test can name the frame that
// panicked rather than matching an anonymous closure.
//
//go:noinline
func panicWith(value any) {
	panic(value)
}

// functionName is the name the runtime gives a function, which is what a stack frame
// carries. Asking the runtime for it keeps the assertion free of a hardcoded module path.
func functionName(fn any) string {
	return runtime.FuncForPC(reflect.ValueOf(fn).Pointer()).Name()
}

func TestPanicErrors(t *testing.T) {
	t.Run("a recovered panic is an error with the stack it came from", func(t *testing.T) {
		var err error

		func() {
			defer RecoverAsError(&err)
			panicOneCallDown()
		}()

		require.Error(t, err)
		assert.Contains(t, err.Error(), "panic (recovered): nested panic")

		// Embedding BaseError is what supplies this, and it is all the formatting path
		// needs: FormatErrorStack walks the unwrap chain looking for the first non-empty
		// frame slice.
		pe := new(panicError)
		require.ErrorAs(t, err, &pe)

		frames := pe.StackFrames()
		require.NotEmpty(t, frames, "a recovered panic must report where it came from")

		// The first frame is the panic site, not the recovery point: the capture skips this
		// package's own frames and the panic machinery with them, so the report starts where
		// the runtime's own panic trace starts. That is what recoverySkip is for, and this
		// is what fails if it is wrong.
		first := frames[0]
		assert.Equal(t, functionName(panicOneCallDown), fmt.Sprintf("%s.%s", first.Package, first.Name),
			"the panic site must be the first frame reported")
	})

	t.Run("a panicked error stays reachable, without losing the stack", func(t *testing.T) {
		sentinel := NewErrorBuilder().SetMessage("the cause").Build()

		var err error

		func() {
			defer RecoverAsError(&err)
			panicWith(sentinel)
		}()

		require.Error(t, err)
		assert.ErrorIs(t, err, sentinel, "the panicked error is the cause, so it is reachable")
		assert.Same(t, sentinel, stderrors.Unwrap(err))

		// Reachable without paying for it: setting the cause must not take the stack from it,
		// which is what Wrap would have done if the sentinel carried frames of its own.
		pe := new(panicError)
		require.ErrorAs(t, err, &pe)
		require.NotEmpty(t, pe.StackFrames())
		assert.Contains(t, fmt.Sprintf("%s.%s", pe.StackFrames()[0].Package, pe.StackFrames()[0].Name),
			functionName(panicWith), "the panic site is still the first frame")

		// And the cause keeps its own, which is what makes reporting ours a choice rather
		// than a loss: it is still there to be read, one unwrap away.
		cause, ok := stderrors.Unwrap(err).(interface{ StackFrames() []goerr.StackFrame })
		require.True(t, ok, "the cause is an error from this package")
		assert.NotEmpty(t, cause.StackFrames(), "the cause keeps its own stack")
	})

	t.Run("a nil dereference reports the function it happened in", func(t *testing.T) {
		var err error

		func() {
			defer RecoverAsError(&err)
			nilDereference()
		}()

		pe := new(panicError)
		require.ErrorAs(t, err, &pe)

		frames := pe.StackFrames()
		require.NotEmpty(t, frames)
		assert.Equal(t, functionName(nilDereference), fmt.Sprintf("%s.%s", frames[0].Package, frames[0].Name),
			"a nil dereference unwinds through extra runtime frames, so the skip has to clear those as well")
	})

	t.Run("no panic leaves the error pointer alone", func(t *testing.T) {
		sample := NewErrorBuilder().SetMessage("sample").Build()
		err := error(sample)

		assert.NotPanics(t, func() {
			defer RecoverAsError(&err)
		})

		assert.Same(t, sample, err)
	})

	t.Run("the formatting path picks the panic error up unchanged", func(t *testing.T) {
		var err error

		func() {
			defer RecoverAsError(&err)
			panicOneCallDown()
		}()

		// %+v is the local-debugging view. It never knows about panicError; it finds the
		// frames through StackFrames.
		assert.Contains(t, fmt.Sprintf("%+v", err), "panic (recovered): nested panic")
		assert.Contains(t, fmt.Sprintf("%+v", err), "panicOneCallDown")
		assert.Contains(t, FormatErrorStack(err), "panicOneCallDown")
	})

	t.Run("deferred through a closure, the panic is not recovered", func(t *testing.T) {
		// recover only sees a panic when the function that deferred it calls recover
		// itself, so this form lets the panic through and the process dies. It is the
		// mistake the doc comment warns about, so it is pinned here in a child process —
		// there is no way to observe it in this one without killing the test run.
		if os.Getenv("MERROBS_PANIC_CLOSURE_CHILD") == "1" {
			var err error

			func() {
				defer func() { RecoverAsError(&err) }()
				panic("through a closure")
			}()

			os.Exit(0) // not reached
		}

		cmd := exec.Command(os.Args[0], "-test.run=TestPanicErrors/deferred_through_a_closure") //nolint:gosec
		cmd.Env = append(os.Environ(), "MERROBS_PANIC_CLOSURE_CHILD=1")
		out, cmdErr := cmd.CombinedOutput()

		require.Error(t, cmdErr, "the panic must escape the closure form")
		assert.Contains(t, string(out), "through a closure")
	})
}
