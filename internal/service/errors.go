// Package service implements the domain logic: auth, users, jobs,
// reports, and analytics, on top of a store adapter.
package service

import "errors"

// ValidationError marks a client-fixable problem; handlers map it to 400.
type ValidationError struct{ Msg string }

func (e *ValidationError) Error() string { return e.Msg }

func validationError(msg string) error {
	return &ValidationError{Msg: msg}
}

// IsValidationError reports whether err or anything it wraps is a ValidationError.
func IsValidationError(err error) bool {
	var ve *ValidationError
	return errors.As(err, &ve)
}
