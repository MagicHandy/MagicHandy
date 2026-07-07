package funscript

import "errors"

// ParseError reports unreadable or malformed funscript input.
type ParseError struct {
	Message string
}

func (e *ParseError) Error() string {
	return e.Message
}

func parseErrorf(format string, args ...any) error {
	return &ParseError{Message: sprintf(format, args...)}
}

// ValidationError reports structural validation failures.
type ValidationError struct {
	Message string
}

func (e *ValidationError) Error() string {
	return e.Message
}

func validationErrorf(format string, args ...any) error {
	return &ValidationError{Message: sprintf(format, args...)}
}

// ErrNotEnoughActions means the timeline is too short after normalization.
var ErrNotEnoughActions = errors.New("not enough valid actions after normalization")
