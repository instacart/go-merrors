package merrors

import (
	"fmt"
	"runtime"
	"strings"

	goerrors "github.com/go-errors/errors"
)

type panicError struct {
	BaseError
	value any
}

// PanicValue returns the value passed to panic.
func (e *panicError) PanicValue() any { return e.value }

func capturePanicFrames() []goerrors.StackFrame {
	pcs := make([]uintptr, 64)
	for {
		n := runtime.Callers(2, pcs)
		if n < len(pcs) {
			pcs = pcs[:n]
			break
		}
		pcs = make([]uintptr, len(pcs)*2)
	}
	iter := runtime.CallersFrames(pcs)
	var frames []goerrors.StackFrame
	seenRuntime := false
	for {
		frame, more := iter.Next()
		pkg, name := splitFunctionName(frame.Function)
		if pkg == "runtime" {
			seenRuntime = true
			if !more {
				break
			}
			continue
		}
		if !seenRuntime && name == "RecoverAsError" {
			if !more {
				break
			}
			continue
		}
		frames = append(frames, goerrors.StackFrame{
			File:           frame.File,
			LineNumber:     frame.Line,
			Name:           name,
			Package:        pkg,
			ProgramCounter: frame.PC + 1,
		})
		if !more {
			break
		}
	}
	return frames
}

func splitFunctionName(full string) (string, string) {
	pkg := ""
	name := full
	if slash := strings.LastIndex(name, "/"); slash >= 0 {
		pkg = name[:slash+1]
		name = name[slash+1:]
	}
	if period := strings.Index(name, "."); period >= 0 {
		pkg += name[:period]
		name = name[period+1:]
	}
	return pkg, strings.ReplaceAll(name, "·", ".")
}

// RecoverAsError converts a recovered panic into an error. Use it directly:
//
//	defer merrors.RecoverAsError(&err)
func RecoverAsError(errp *error) {
	if recovered := recover(); recovered != nil {
		var cause error
		if err, ok := recovered.(error); ok {
			cause = err
		}
		message := fmt.Sprintf("panic (recovered): %v", recovered)
		pe := &panicError{
			BaseError: NewBaseError(WithMessage(message), WithStackFrames(capturePanicFrames())),
			value:     recovered,
		}
		if cause != nil {
			pe.cause = cause
		}
		*errp = pe
	}
}
