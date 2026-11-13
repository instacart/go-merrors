# Mashed Errors (merrors)

Library for creating custom errors with stack traces.

It is built on top of the [`github.com/go-errors/errors`](https://github.com/go-errors/errors)
package.

Goals:

- Be able to create custom errors and extend functionality (for example, add new fields).
- Be able to wrap other errors and add contextual information to them.
- Support structured logging with stack traces.

Pre-1.0, maintained on a best-effort basis. See [Project status](#project-status).

## Usage

### Creating an error with a stack trace

```go
err := merrors.New("could not read manifest")
fmt.Printf("%+v\n", err)
```

`%+v` prints the message followed by the captured stack. Use it for local debugging; for
production logs, use `FormatErrorStack` as described below.

### Adding context by wrapping

```go
inner := merrors.New("innerError")
err := merrors.Wrap(inner, merrors.WithMessage("error calling methodA"))

fmt.Println(err.Error())
// innerError
// error calling methodA
```

Use `WithMessagef` with `Wrap` for a formatted message. `Errorf` preserves a wrapped cause when
the format contains `%w`.

### Logging an error

This package does not depend on a logger. Use `FormatErrorStack` to build the message and stack
trace for a log line. With zap, emit them as structured fields:

```go
import (
    "go.uber.org/zap"
    "go.uber.org/zap/zapcore"

    "github.com/instacart/go-merrors/merrors"
)

func zapError(err error) zap.Field {
    return zap.Object("error", zapcore.ObjectMarshalerFunc(func(enc zapcore.ObjectEncoder) error {
        enc.AddString("message", err.Error())
        enc.AddString("stack", merrors.FormatErrorStack(err))
        return nil
    }))
}

logger := zap.NewExample()
defer logger.Sync()
logger.Error("request failed", zapError(err))
```

`zap.NewExample()` is a ready-to-run development logger; production code normally injects the
application's configured `*zap.Logger`.

For console or plain-text logging, use `zap.String("error", merrors.FormatErrorStack(err))`.

### Creating a sentinel error

`NewSentinel` creates a stackless error for identity checks:

```go
var ErrNotFound = merrors.NewSentinel("not_found")

if errors.Is(err, ErrNotFound) {
    // handle the sentinel case
}
```

## Custom error types

Embed `BaseError` by value and build the message in a constructor:

```go
type CustomError struct {
    merrors.BaseError
    Source string
}

func NewCustomError(source, message string, cause error) *CustomError {
    return &CustomError{
        BaseError: merrors.NewBaseError(
            merrors.Wrapping(cause),
            merrors.WithMessagef("%s (source = %s)", message, source),
        ),
        Source: source,
    }
}

err := NewCustomError("billing", "charge failed", cause)
```

Call `NewBaseError` only from a constructor. It captures the stack at the constructor's caller; use
`SkipFrames(n)` for deeper helpers.

If you also want callers to be able to override options while keeping custom formatting, add a
message combiner after the caller options:

```go
import "fmt"

type CustomError struct {
    merrors.BaseError
    Source string
}

func NewCustomError(source, message string, opts ...merrors.Option) *CustomError {
    opts = append([]merrors.Option{merrors.WithMessage(message)}, opts...)
    opts = append(opts, merrors.WithMessageCombiner(func(wrapped, current string) string {
        return merrors.DefaultMessageCombiner(
            wrapped,
            fmt.Sprintf("CustomError: %s (source = %s)", current, source),
        )
    }))
    return &CustomError{
        BaseError: merrors.NewBaseError(opts...),
        Source:    source,
    }
}
```

Appending the combiner last makes the custom formatting invariant; caller message options still
apply before it.

### Using custom types safely

- Embed `BaseError` by value.
- Do not implement `Error()`, `Format`, or `StackFrames` on the custom type. `BaseError` owns them,
  so `err.Error()`, `%v`, `%+v`, and `FormatErrorStack` agree.
- Render custom fields into the message at construction. Keep them as struct fields too when
  callers need them via `errors.As`.
- Call `CheckErrorType` from tests of custom error types to catch an accidental method override:

  ```go
  if err := merrors.CheckErrorType(customErr); err != nil {
      t.Fatal(err)
  }
  ```

- If a constructor needs explicit stack frames, pass them to `NewBaseError` with
  `WithStackFrames(frames)` instead of overriding `StackFrames`.

For an advanced message clone, copy the custom type and delegate the base error to
`CloneBaseErrorWithAppendMessage`:

```go
func (e *CustomError) WithAppendMessage(msg string) *CustomError {
    clone := *e
    clone.BaseError = merrors.CloneBaseErrorWithAppendMessage(e.BaseError, msg)
    return &clone
}
```

`clone := *e` preserves the custom fields, so nothing is lost.

## Options

| Option | Effect |
| --- | --- |
| `WithMessage(message)` | Sets this error's message. Last message option wins. |
| `WithMessagef(format, args...)` | Formatted message; last message option wins. |
| `WithMessageCombiner(combiner)` | Combines wrapped and current messages; last combiner wins. |
| `Wrapping(err)` | Sets the cause and reuses its stack when available. |
| `OmitWrappedMessage()` | Keeps only this error's message. |
| `SkipFrames(n)` | Skips extra frames when capturing a new stack. |
| `WithoutStack()` | Disables capturing a new stack. |
| `WithStackFrames(frames)` | Supplies an explicit stack, for advanced custom overrides. |

`DefaultMessageCombiner` puts the wrapped message first and the current message on a new line,
omitting empty messages. Pass `WithMessageCombiner` to customize that behavior.

`NewBaseError` captures the stack at the caller of the constructor that calls it. If no message is
supplied, this layer's message is empty and does not add a blank line.

## Formatting

`FormatErrorStack` returns the complete message followed by the first stack found in the chain.
`%+v` uses the same stack formatting.

```go
fmt.Printf("%+v\n", err)
logger.WithField("error", merrors.FormatErrorStack(err)).Error("request failed")
```

`Format` preserves normal string formatting flags, width, and precision for `%s`, `%q`, `%x`, and
`%X`.

## Wrapping and stacks

The wrapped cause remains in the chain so `errors.Is` and `errors.As` behave normally. When the
cause already has a stack, the wrapper reuses it instead of capturing a replacement stack. A
reporting tool that walks `Unwrap` may therefore see the same origin stack at more than one layer;
this is intentional for now and avoids reporting the wrapper location as a new failure site.

## Panic recovery

Use `RecoverAsError` directly in a deferred call:

```go
func work() (err error) {
    defer merrors.RecoverAsError(&err)
    panic("boom")
}

fmt.Println(work().Error())
// panic (recovered): boom
```

Recovery captures the panic site's stack. If the panic value is an error, it remains reachable with
`errors.Is`/`errors.As` while the recovered error gets its own recovery stack.

## API

```go
type Error interface {
    error
    StackFrames() []errors.StackFrame
    // contains unexported sealing method
}

type MessageCombiner func(wrappedMessage, currentMessage string) string

func DefaultMessageCombiner(wrappedMessage, currentMessage string) string
func WithMessageCombiner(combiner MessageCombiner) Option
func New(message string) error
func Errorf(format string, a ...any) error
func Wrap(err error, opts ...Option) error
func NewSentinel(message string) error
func RecoverAsError(errp *error)
func CheckErrorType(err Error) error
func FormatErrorStack(err error) string
func As(err error, target any) bool
func Is(err, target error) bool
func Unwrap(err error) error
```

The convenience constructors return plain `error`, so they compose like any other Go error. If
you need the captured stack directly, cast the result to `merrors.Error`:

```go
err := merrors.New("message")
if stacked, ok := err.(merrors.Error); ok {
    frames := stacked.StackFrames()
}
```

`FormatErrorStack(err)` is the simpler option when you only need the formatted stack.

`CloneBaseErrorWithAppendMessage` is an advanced helper for building a new error of the same
custom type with an adjusted message; most callers should build the message during construction or
use `Wrap` to add context.

## Development

Requires Go 1.25 or later.

```bash
make test
```

## Project status

A variant of this library is used at Instacart. We're publishing it so others can use, adapt, and
build on the ideas.

This is not an officially supported Instacart product and comes with no commitment to ongoing
maintenance. Fixes and improvements are made on a best-effort basis, and the repository may be
archived in the future.

The API is pre-1.0 and may change between minor versions; breaking changes will be noted in the
release notes.

Licensed under the Apache License 2.0. See [LICENSE](LICENSE).
