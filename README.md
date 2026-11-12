# Mashed Errors (merrors)

Library for creating custom errors with stacktraces.

It is built on top of the https://github.com/go-errors/errors package.

Goals:
- Be able to create custom errors and extend functionality (like add new fields).
- Be able to wrap other errors and add contextual information to them.
- Support structured logging with stacktraces (for example, Datadog).

Pre-1.0, maintained on a best-effort basis. See [Project status](#project-status).

## Usage

### Creating an error with stack trace.

Code below creates an error and prints it for debugging.
```go
err := merrors.NewErrorBuilder().SetMessage("message").Build()
fmt.Printf("%+v\n", err)
```

The code above prints:
```
message
main.main
	main.go:22
runtime.goexit
	src/runtime/asm_amd64.s:1695
```

Note: The formatting `%+v` instructs the error to output its stacktrace in addition to its message. You should only use it for local debugging as for production you should instead follow the instructions in `Logging an error` section.

Runnable: `Example` in [`example/basic_test.go`](example/basic_test.go).

### Create a new error from an existing one adding more details

```go
import (
	"fmt"

	"github.com/instacart/go-merrors/merrors"
)

func methodA() error {
	return merrors.NewErrorBuilder().SetMessage("innerError").Build()
}

func methodB() error {
	err := methodA()
	if err != nil {
		// When wrapping another error, SetMessage adds the message in the end.
		return merrors.NewErrorBuilder().Wrap(err).SetMessage("error calling methodA").Build()
	}
	return nil
}

func main() {
    // ...
	fmt.Printf("%+v\n", methodB())
    // ...
}
```

You will get:
```
innerError
error calling methodA
main.methodA
	main.go:20
main.methodB
	main.go:24
main.main
	main.go:39
runtime.goexit
	src/runtime/asm_amd64.s:1695
```

Runnable: `ExampleNewErrorBuilder` in [`example/basic_test.go`](example/basic_test.go).

### Logging an error

This package does not depend on a logger. Use `merrors.FormatErrorStack` to build the error
message and stacktrace for a log line.

Runnable: `TestFormatErrorStack` in [`example/basic_test.go`](example/basic_test.go).

### Creating a minimal custom error

Below is an example of a custom-defined error.

```go
type CustomError struct {
	merrors.BaseError[*CustomError]
}

type CustomErrorBuilder struct {
	merrors.BaseErrorBuilder[*CustomErrorBuilder, *CustomError]
}

func NewCustomErrorBuilder() *CustomErrorBuilder {
	return merrors.InitBaseErrorBuilder(&CustomErrorBuilder{}, func() *CustomError { return &CustomError{} })
}
```


Runnable: `ExampleInitBaseErrorBuilder` in [`example/custom_error_test.go`](example/custom_error_test.go) — it also pins what wrapping `nil` returns.

### Creating a custom error with extra properties and custom error messages.

Below is an example of a custom-defined error with extra properties.

It carries some extra properties (source) and has a custom implementation for the Error method that adds some extra message.

```go
type CustomError struct {
	merrors.BaseError[*CustomError]

	source string
}

func (c *CustomError) Error() string {
	return "CustomError: " + c.BaseError.Error() + " :/CustomError"
}

func (c *CustomError) Source() string {
	return c.source
}

func (c *CustomError) PopulateClone(clone *CustomError) {
	c.BaseError.PopulateClone(clone)
	clone.source = c.source
}

func (b *CustomErrorBuilder) PopulateBuilderResult(result *CustomError) {
	b.SetMessage(b.GetMessage() + " (source = " + b.source + ")")
	result.source = b.source
	b.BaseErrorBuilder.PopulateBuilderResult(result)
}

type CustomErrorBuilder struct {
	merrors.BaseErrorBuilder[*CustomErrorBuilder, *CustomError]

	source string
}

func (b *CustomErrorBuilder) SetSource(source string) *CustomErrorBuilder {
	b.source = source
	return b
}

func NewCustomErrorBuilder() *CustomErrorBuilder {
	return merrors.InitBaseErrorBuilder(&CustomErrorBuilder{}, func() *CustomError { return &CustomError{} })
}
```

Example usage:
```go
customErr := NewCustomErrorBuilder().Wrap(err).SetSource("source").SetMessage("custom error message").Build()
fmt.Printf("%+v\n", customErr)
```

Running the above gets you:
```
CustomError: message
custom error message (source = source) :/CustomError
main.main
	main.go:38
runtime.goexit
	src/runtime/asm_amd64.s:1695
```

Runnable: `ExampleInitBaseErrorBuilder_withProperties` in [`example/custom_error_test.go`](example/custom_error_test.go).

### Recovering a panic

`RecoverAsError` turns a recovered panic into an error from this package, so it is logged
with a stacktrace like everything else.

```go
func work() (err error) {
	defer merrors.RecoverAsError(&err)
	panic("boom")
	return nil
}

err := work()
fmt.Printf("%+v\n", err)
```

The code above prints:
```
panic (recovered): boom
main.work
	main.go:22
main.main
	main.go:26
runtime.goexit
	src/runtime/asm_amd64.s:1693
```

The first frame is the function that panicked, not the one that recovered it. Runtime frames
are skipped by name rather than by count, because different panics unwind through different
numbers of runtime frames.

Three things to know before using it:

- **Defer it directly** — `defer merrors.RecoverAsError(&err)` — and not through a closure.
  `recover` only sees a panic when the function that deferred it calls `recover` itself, so
  `defer func() { merrors.RecoverAsError(&err) }()` lets the panic through and the process
  dies.
- **The panic value is in the message, and unwrapped to when it is an error.** A panicked
  error stays reachable with `errors.Is` and `errors.As` from the standard library; anything else is in the message.
  It is deliberately not attached with `Wrap`, which takes the stack from the wrapped error
  whenever that error has frames of its own — so a panicked error would report where it was
  created rather than where it was raised, which is the opposite of the point.
- **The stack reported is the panic site**, and a panicked error that has a stack of its own
  keeps it — unwrapping reaches it. Only one of the two can be in the report, because
  `FormatErrorStack` stops at the first non-empty frames in the chain, and the panic site is
  the one that cannot be recovered afterwards: it is only capturable where it is captured.

If there is no panic, the error pointer keeps whatever it held.

Runnable: `ExampleRecoverAsError` and `TestRecoverAsErrorKeepsExisting` in [`example/recover_test.go`](example/recover_test.go).

## Development

Requires Go 1.25 or later. Run the tests with:

```
make test
```

Every file under `example/` is a test. The directory has no non-test Go files, so it cannot
be imported, and `go test ./...` still compiles and runs everything in it — which is what keeps
the examples above from drifting out of date.

## Project status

A variant of this library is used at Instacart. We're publishing it so others
can use, adapt, and build on the ideas.

This is not an officially supported Instacart product and comes with no
commitment to ongoing maintenance. Fixes and improvements are made on a
best-effort basis, and the repository may be archived in the future.

The API is pre-1.0 and may change between minor versions; breaking changes
will be noted in the release notes.

Licensed under the Apache License 2.0. See [LICENSE](LICENSE).
