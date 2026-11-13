// Package merrors provides composition-based errors with stack traces.
//
// Custom error types embed BaseError by value and render their fields into the
// message at construction. The convenience constructors return plain error
// values; cast to Error when direct access to StackFrames is required.
//
// Wrapping preserves the original error in the chain and reuses its stack.
// FormatErrorStack and the %+v formatter render the first stack found in the
// chain.
package merrors
