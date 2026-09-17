package verify

import (
	"errors"
	"fmt"
	"strings"

	"github.com/vertex-language/vsc/internal/sil"
)

// Verification error sentinels.
var (
	ErrNoEntry            = errors.New("function has no entry block")
	ErrTerminator         = errors.New("block does not end in a terminator")
	ErrUnreachable        = errors.New("block is unreachable")
	ErrEntryTarget        = errors.New("entry block is a branch target")
	ErrDominance          = errors.New("use is not dominated by its definition")
	ErrBranchArity        = errors.New("branch arguments do not match the destination")
	ErrSignature          = errors.New("function body does not match its type")
	ErrLeak               = errors.New("owned value is not consumed on all paths")
	ErrDoubleConsume      = errors.New("owned value is consumed twice")
	ErrUseAfterConsume    = errors.New("value is used after it is consumed")
	ErrConsumedGuaranteed = errors.New("guaranteed value is consumed")
	ErrBorrowNotEnded     = errors.New("borrow is not ended on all paths")
	ErrUseOutsideBorrow   = errors.New("borrowed value is used outside its scope")
	ErrOwnership          = errors.New("ownership is wrong for the value")
	ErrStoreType          = errors.New("stored value is not what the address holds")
	ErrStage              = errors.New("instruction is not allowed at this stage")
)

// An Error describes a specific verification failure with its location.
type Error struct {
	Func   string // "" at module scope
	Block  string // "" when the fault is the function's own
	Inst   int    // index into the block's instructions; -1 for none
	Op     sil.Op // the zero Op when no single instruction is at fault
	Value  int    // the %n at fault; -1 for none
	Detail string
	Err    error // one of the sentinels above
}

func (e *Error) Error() string {
	var b strings.Builder
	b.WriteString("verify: ")
	if e.Func != "" {
		b.WriteString("@" + e.Func)
		if e.Block != "" {
			b.WriteString(" " + e.Block)
			if e.Inst >= 0 {
				fmt.Fprintf(&b, " #%d", e.Inst)
			}
		}
		b.WriteString(": ")
	}
	if e.Op != "" {
		b.WriteString(string(e.Op) + ": ")
	}
	b.WriteString(e.Err.Error())
	if e.Value >= 0 {
		fmt.Fprintf(&b, " (%%%d)", e.Value)
	}
	if e.Detail != "" {
		b.WriteString(": " + e.Detail)
	}
	return b.String()
}

func (e *Error) Unwrap() error { return e.Err }

// Errors is a collection of verification failures.
type Errors []*Error

func (es Errors) Error() string {
	switch len(es) {
	case 0:
		return "verify: no errors"
	case 1:
		return es[0].Error()
	}
	var b strings.Builder
	fmt.Fprintf(&b, "verify: %d errors:", len(es))
	for _, e := range es {
		b.WriteString("\n  " + e.Error())
	}
	return b.String()
}

func (es Errors) Is(target error) bool {
	for _, e := range es {
		if errors.Is(e, target) {
			return true
		}
	}
	return false
}

// collector gathers faults as they are found.
type collector struct {
	fn   *sil.Func
	errs Errors

	twice, after error // Sentinels for double-use and use-after errors during walk
}

func (c *collector) at(b *sil.Block, i int, op sil.Op, v *sil.Value, err error, detail string) {
	e := &Error{Inst: -1, Value: -1, Err: err, Detail: detail, Op: op}
	if c.fn != nil {
		e.Func = c.fn.Name()
	}
	if b != nil {
		e.Block = b.Label()
		e.Inst = i
	}
	if v != nil {
		e.Value = v.ID()
	}
	c.errs = append(c.errs, e)
}

func (c *collector) fnErr(err error, detail string) {
	c.at(nil, -1, "", nil, err, detail)
}
