package verify

import (
	"errors"
	"fmt"
	"strings"

	"github.com/vertex-language/vsc/vil"
)

// The sentinels: the faults a finished module reveals.
//
// Two groups. The structural ones are what any SSA IR must hold —
// terminators, dominance, branch arities — and they are checked
// first, because the ownership rules are stated over a graph and a
// malformed graph makes them meaningless.
//
// The ownership ones are the reason this IR exists. They are two
// rules, and everything ARC does rests on them being true.
var (
	// ErrNoEntry is a function with blocks but no entry block, which
	// only an empty block list can be.
	ErrNoEntry = errors.New("function has no entry block")

	// ErrTerminator is a block that does not end in one, or that has
	// one anywhere but last.
	ErrTerminator = errors.New("block does not end in a terminator")

	// ErrUnreachable is a block no path from the entry reaches. Only
	// the entry block may be reachable from nothing.
	ErrUnreachable = errors.New("block is unreachable")

	// ErrEntryTarget is the entry block named as a branch target. Its
	// arguments are the function's parameters and are passed by the
	// caller, so nothing inside may pass them again.
	ErrEntryTarget = errors.New("entry block is a branch target")

	// ErrDominance is a use of a value the definition does not
	// dominate: a path reaches the use without passing the def.
	ErrDominance = errors.New("use is not dominated by its definition")

	// ErrBranchArity is a branch whose argument list does not match
	// the destination block's parameters, in count or in type.
	ErrBranchArity = errors.New("branch arguments do not match the destination")

	// ErrSignature is an entry block whose arguments are not the
	// function's parameters, or a return whose operand is not the
	// function's result.
	ErrSignature = errors.New("function body does not match its type")

	// ErrLeak is rule one's first half: an owned value that is still
	// alive where the function ends. Something owns it and nothing
	// will ever release it.
	ErrLeak = errors.New("owned value is not consumed on all paths")

	// ErrDoubleConsume is rule one's second half: a path on which an
	// owned value is consumed twice.
	ErrDoubleConsume = errors.New("owned value is consumed twice")

	// ErrUseAfterConsume is a use of a value on a path where it has
	// already been consumed.
	ErrUseAfterConsume = errors.New("value is used after it is consumed")

	// ErrConsumedGuaranteed is rule two's first half: a borrowed
	// value consumed by something that does not own it.
	ErrConsumedGuaranteed = errors.New("guaranteed value is consumed")

	// ErrBorrowNotEnded is a borrow scope that some path leaves
	// without closing.
	ErrBorrowNotEnded = errors.New("borrow is not ended on all paths")

	// ErrUseOutsideBorrow is a use of a borrowed value after the
	// scope that produced it has ended.
	ErrUseOutsideBorrow = errors.New("borrowed value is used outside its scope")

	// ErrOwnership is a value whose ownership does not match what its
	// type or its instruction allows: a trivial value that is owned,
	// or a destroy of something that owns nothing.
	ErrOwnership = errors.New("ownership is wrong for the value")

	// ErrStage is a rule the module's stage requires and it does not
	// hold: raw-only instructions in a canonical module, or a
	// canonical module whose functions still carry mark_uninitialized.
	ErrStage = errors.New("instruction is not allowed at this stage")
)

// An Error is one fault, located as precisely as the fault allows.
type Error struct {
	Func   string // "" at module scope
	Block  string // "" when the fault is the function's own
	Inst   int    // index into the block's instructions; -1 for none
	Op     vil.Op // the zero Op when no single instruction is at fault
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

// Errors is every fault one run found, in the order it found them.
//
// All of them, not the first: a verifier reads a module that is
// already finished, the faults are independent, and reporting one at
// a time would make fixing a module a sequence of runs. errors.Is
// reaches every sentinel in the list.
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
	fn   *vil.Func
	errs Errors

	// twice and after are the sentinels the ownership walk reports
	// with, which differ between the two rules it is shared by.
	twice, after error
}

func (c *collector) at(b *vil.Block, i int, op vil.Op, v *vil.Value, err error, detail string) {
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
