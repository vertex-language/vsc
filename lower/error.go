package lower

import (
	"errors"
	"fmt"

	"github.com/vertex-language/vsc/vil"
)

// The reasons lowering stops.
var (
	// ErrStage is a module that has not been through vil/pass.
	ErrStage = errors.New("lower: the module is not in the lowered stage")
	// ErrUnsupported is an instruction this package cannot yet translate.
	ErrUnsupported = errors.New("lower: unsupported")
	// ErrType is a type with no machine representation here yet.
	ErrType = errors.New("lower: no machine type")
	// ErrBuiltin is a Builtin.* function with no VIR verb.
	ErrBuiltin = errors.New("lower: unknown builtin")
	// ErrIR is a failure reported by the ir package itself.
	ErrIR = errors.New("lower: ir")
)

// An Error says what could not be lowered and where.
type Error struct {
	Err  error
	Func string
	Op   vil.Op
	What string
}

func (e *Error) Error() string {
	at := e.Func
	if e.Op != "" {
		at += ": " + string(e.Op)
	}
	if at == "" {
		return e.Err.Error()
	}
	if e.What == "" {
		return fmt.Sprintf("%s: in %s", e.Err, at)
	}
	return fmt.Sprintf("%s: %s: in %s", e.Err, e.What, at)
}

func (e *Error) Unwrap() error { return e.Err }
