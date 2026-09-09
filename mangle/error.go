package mangle

import (
	"errors"
	"fmt"
)

// The reasons a declaration has no symbol here.
var (
	// ErrUnsupported is a declaration whose mangling is not written yet.
	ErrUnsupported = errors.New("mangle: unsupported")
	// ErrType is a type with no spelling here yet.
	ErrType = errors.New("mangle: no spelling for type")
	// ErrName is an identifier this scheme cannot carry.
	ErrName = errors.New("mangle: bad identifier")
)

// An Error says what could not be spelled.
type Error struct {
	Err  error
	What string
}

func (e *Error) Error() string {
	if e.What == "" {
		return e.Err.Error()
	}
	return fmt.Sprintf("%s: %s", e.Err, e.What)
}

func (e *Error) Unwrap() error { return e.Err }

func fail(err error, what string) error { return &Error{Err: err, What: what} }
