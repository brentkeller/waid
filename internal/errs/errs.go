// Package errs holds the error types the CLI maps onto exit codes. It sits below every other
// package so a command, the config loader and the CLI itself can all raise the same error without
// depending on one another.
package errs

import "fmt"

// UserError is an error caused by the user's input or environment. The CLI maps it to exit code 1
// and prints only its message; every other error is internal and exits 2 with a stack.
type UserError struct {
	Message string
	// Candidates holds alternatives to show when the input was ambiguous.
	Candidates []string
}

func (e *UserError) Error() string { return e.Message }

// Userf builds a UserError from a format string.
func Userf(format string, args ...any) *UserError {
	return &UserError{Message: fmt.Sprintf(format, args...)}
}

// Ambiguous builds a UserError that lists the alternatives the input matched.
func Ambiguous(message string, candidates []string) *UserError {
	return &UserError{Message: message, Candidates: candidates}
}
