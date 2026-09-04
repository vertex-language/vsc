package verify

import (
	"errors"
	"testing"

	"github.com/vertex-language/vsc/types"
	"github.com/vertex-language/vsc/vil"
)

// One case per rule, each a function built to break exactly one of
// them, and each expected to fail with the sentinel that names it.
// The sound cases come first, because a verifier that rejects correct
// code is worse than one that misses a fault.

func classType(name string) types.Type {
	return types.NewNamed(name, "", &types.Class{Name: name})
}

var (
	boxT = vil.Object(classType("Box"))
	intT = vil.Object(types.Typ[types.Int])
)

// fn starts a function in ownership form.
func fn(name string) *vil.Func {
	m := vil.NewModule("t", vil.StageRaw)
	return m.Func(name).SetLinkage(vil.Hidden).SetAttr("ossa")
}

// wants runs the verifier and requires exactly the named fault.
func wants(t *testing.T, f *vil.Func, sentinel error) {
	t.Helper()
	err := Func(f)
	if err == nil {
		t.Fatalf("%s: verified clean, want %v", f.Name(), sentinel)
	}
	if !errors.Is(err, sentinel) {
		t.Fatalf("%s: got %v, want %v", f.Name(), err, sentinel)
	}
}

// clean requires the function to verify.
func clean(t *testing.T, f *vil.Func) {
	t.Helper()
	if err := Func(f); err != nil {
		t.Fatalf("%s: %v", f.Name(), err)
	}
}

// ---- sound ----

// TestBorrowedParameterVerifies is the shape of `func borrows(_ b:
// Box) -> Int`: nothing is owned, nothing is consumed, and the
// verifier has nothing to say.
func TestBorrowedParameterVerifies(t *testing.T) {
	f := fn("borrows")
	b := f.Param(boxT, vil.ParamGuaranteed)
	f.SetResult(intT, vil.ResultUnowned)

	bb := f.Entry()
	addr := bb.RefElementAddr(b, "Box.n", intT)
	bb.Return(bb.Load(addr, "trivial"))
	clean(t, f)
}

// TestOwnedParameterVerifies is `consumes`: borrow it, copy it, end
// the borrow, destroy what was given, return what was made. Every
// owned value consumed exactly once.
func TestOwnedParameterVerifies(t *testing.T) {
	f := fn("consumes")
	b := f.Param(boxT, vil.ParamOwned)
	f.SetResult(boxT, vil.ResultOwned)

	bb := f.Entry()
	borrow := bb.BeginBorrow(b)
	copied := bb.CopyValue(borrow)
	bb.EndBorrow(borrow)
	bb.DestroyValue(b)
	bb.Return(copied)
	clean(t, f)
}

// TestBranchesVerify consumes a value on both arms of a branch, which
// is the shape a real function has and the one the dataflow has to
// get right.
func TestBranchesVerify(t *testing.T) {
	f := fn("branches")
	b := f.Param(boxT, vil.ParamOwned)
	cond := f.Param(vil.Object(vil.BuiltinInt1), vil.ParamUnowned)
	f.SetResult(intT, vil.ResultUnowned)

	entry, yes, no := f.Entry(), f.Block(), f.Block()
	entry.CondBr(cond, yes, nil, no, nil)

	yes.DestroyValue(b)
	yes.Return(yes.IntegerLiteral(intT, 1))

	no.DestroyValue(b)
	no.Return(no.IntegerLiteral(intT, 2))
	clean(t, f)
}

// ---- structure ----

func TestNoTerminator(t *testing.T) {
	f := fn("noterm")
	f.SetResult(intT, vil.ResultUnowned)
	f.Entry().IntegerLiteral(intT, 1)
	wants(t, f, ErrTerminator)
}

func TestUnreachableBlock(t *testing.T) {
	f := fn("unreachable")
	f.SetResult(intT, vil.ResultUnowned)
	entry, orphan := f.Entry(), f.Block()
	entry.Return(entry.IntegerLiteral(intT, 1))
	orphan.Return(orphan.IntegerLiteral(intT, 2))
	wants(t, f, ErrUnreachable)
}

func TestEntryIsNobodysTarget(t *testing.T) {
	f := fn("loopback")
	f.SetResult(intT, vil.ResultUnowned)
	entry, second := f.Entry(), f.Block()
	entry.Br(second)
	second.Br(entry)
	wants(t, f, ErrEntryTarget)
}

func TestBranchArity(t *testing.T) {
	f := fn("arity")
	f.SetResult(intT, vil.ResultUnowned)
	entry, dest := f.Entry(), f.Block()
	dest.Arg(intT, vil.None)
	entry.Br(dest) // passes nothing to a block that takes one
	dest.Return(dest.Args()[0])
	wants(t, f, ErrBranchArity)
}

func TestBranchArgumentType(t *testing.T) {
	f := fn("argtype")
	b := f.Param(boxT, vil.ParamGuaranteed)
	f.SetResult(intT, vil.ResultUnowned)
	entry, dest := f.Entry(), f.Block()
	dest.Arg(intT, vil.None)
	entry.Br(dest, b) // a Box where an Int is declared
	dest.Return(dest.Args()[0])
	wants(t, f, ErrBranchArity)
}

func TestSignatureMismatch(t *testing.T) {
	f := fn("sig")
	b := f.Param(boxT, vil.ParamGuaranteed)
	f.SetResult(intT, vil.ResultUnowned)
	f.Entry().Return(b) // returns a Box where the type says Int
	wants(t, f, ErrSignature)
}

func TestDominance(t *testing.T) {
	f := fn("dominance")
	cond := f.Param(vil.Object(vil.BuiltinInt1), vil.ParamUnowned)
	f.SetResult(intT, vil.ResultUnowned)

	entry, yes, join := f.Entry(), f.Block(), f.Block()
	entry.CondBr(cond, yes, nil, join, nil)
	v := yes.IntegerLiteral(intT, 1) // defined on one path only
	yes.Br(join)
	join.Return(v)
	wants(t, f, ErrDominance)
}

// ---- rule one ----

func TestLeak(t *testing.T) {
	f := fn("leak")
	f.Param(boxT, vil.ParamOwned) // never consumed
	f.SetResult(intT, vil.ResultUnowned)
	bb := f.Entry()
	bb.Return(bb.IntegerLiteral(intT, 0))
	wants(t, f, ErrLeak)
}

func TestDoubleConsume(t *testing.T) {
	f := fn("double")
	b := f.Param(boxT, vil.ParamOwned)
	f.SetResult(intT, vil.ResultUnowned)
	bb := f.Entry()
	bb.DestroyValue(b)
	bb.DestroyValue(b)
	bb.Return(bb.IntegerLiteral(intT, 0))
	wants(t, f, ErrDoubleConsume)
}

func TestUseAfterConsume(t *testing.T) {
	f := fn("useafter")
	b := f.Param(boxT, vil.ParamOwned)
	f.SetResult(intT, vil.ResultUnowned)
	bb := f.Entry()
	bb.DestroyValue(b)
	addr := bb.RefElementAddr(b, "Box.n", intT) // read after the destroy
	bb.Return(bb.Load(addr, "trivial"))
	wants(t, f, ErrUseAfterConsume)
}

// TestConsumedOnOnePath is the fault a branch makes: destroyed on one
// arm and not the other, which leaks down one path and would double
// free if the other ever caught up.
func TestConsumedOnOnePath(t *testing.T) {
	f := fn("onepath")
	b := f.Param(boxT, vil.ParamOwned)
	cond := f.Param(vil.Object(vil.BuiltinInt1), vil.ParamUnowned)
	f.SetResult(intT, vil.ResultUnowned)

	entry, yes, join := f.Entry(), f.Block(), f.Block()
	entry.CondBr(cond, yes, nil, join, nil)
	yes.DestroyValue(b)
	yes.Br(join)
	join.Return(join.IntegerLiteral(intT, 0))
	wants(t, f, ErrLeak)
}

// ---- rule two ----

func TestBorrowNotEnded(t *testing.T) {
	f := fn("openborrow")
	b := f.Param(boxT, vil.ParamOwned)
	f.SetResult(intT, vil.ResultUnowned)
	bb := f.Entry()
	bb.BeginBorrow(b) // never ended
	bb.DestroyValue(b)
	bb.Return(bb.IntegerLiteral(intT, 0))
	wants(t, f, ErrBorrowNotEnded)
}

func TestUseOutsideBorrow(t *testing.T) {
	f := fn("outsideborrow")
	b := f.Param(boxT, vil.ParamOwned)
	f.SetResult(intT, vil.ResultUnowned)
	bb := f.Entry()
	borrow := bb.BeginBorrow(b)
	bb.EndBorrow(borrow)
	addr := bb.RefElementAddr(borrow, "Box.n", intT) // after the scope closed
	bb.Load(addr, "trivial")
	bb.DestroyValue(b)
	bb.Return(bb.IntegerLiteral(intT, 0))
	wants(t, f, ErrUseOutsideBorrow)
}

// TestConsumedWhileBorrowed is the fault that makes a borrow unsound:
// the value the borrow was taken from is destroyed while the borrow
// is still open, so the borrow points at freed memory.
func TestConsumedWhileBorrowed(t *testing.T) {
	f := fn("consumedwhileborrowed")
	b := f.Param(boxT, vil.ParamOwned)
	f.SetResult(intT, vil.ResultUnowned)
	bb := f.Entry()
	borrow := bb.BeginBorrow(b)
	bb.DestroyValue(b) // while borrow is live
	bb.EndBorrow(borrow)
	bb.Return(bb.IntegerLiteral(intT, 0))
	wants(t, f, ErrUseAfterConsume)
}

// TestGuaranteedIsNotConsumed: a borrowed value belongs to someone
// else, and returning it as owned gives away what was not ours.
func TestOwnershipKind(t *testing.T) {
	f := fn("trivialowned")
	f.SetResult(intT, vil.ResultUnowned)
	bb := f.Entry()
	n := bb.IntegerLiteral(intT, 1)
	bb.DestroyValue(n) // an Int owns nothing
	bb.Return(n)
	wants(t, f, ErrOwnership)
}

// TestDeclarationsAreNotChecked: a function with no body cannot be
// wrong about one.
func TestDeclaration(t *testing.T) {
	m := vil.NewModule("t", vil.StageRaw)
	f := m.Func("external")
	f.SetResult(intT, vil.ResultUnowned)
	if err := Func(f); err != nil {
		t.Errorf("a declaration should verify: %v", err)
	}
}

// TestModuleReportsEveryFunction: the module entry point returns
// every fault, not the first function's.
func TestModuleCollectsAll(t *testing.T) {
	m := vil.NewModule("t", vil.StageRaw)
	for _, name := range []string{"a", "b"} {
		f := m.Func(name).SetAttr("ossa")
		f.Param(boxT, vil.ParamOwned)
		f.SetResult(intT, vil.ResultUnowned)
		bb := f.Entry()
		bb.Return(bb.IntegerLiteral(intT, 0))
	}
	err := Module(m)
	var errs Errors
	if !errors.As(err, &errs) {
		t.Fatalf("got %T, want Errors", err)
	}
	if len(errs) != 2 {
		t.Errorf("got %d faults, want one per function: %v", len(errs), err)
	}
}

// TestGuaranteedIsNotConsumed: a borrowed value belongs to whoever
// lent it. Giving it away is the other half of rule two.
func TestGuaranteedIsNotConsumed(t *testing.T) {
	f := fn("givesawayaborrow")
	b := f.Param(boxT, vil.ParamGuaranteed)
	f.SetResult(boxT, vil.ResultOwned)
	f.Entry().Return(b) // returns what it does not own
	wants(t, f, ErrConsumedGuaranteed)
}

// TestStage: mark_uninitialized exists to be removed, and a canonical
// module is one the pass that removes it has already run over.
func TestStage(t *testing.T) {
	m := vil.NewModule("t", vil.StageCanonical)
	f := m.Func("stale").SetAttr("ossa")
	f.SetResult(intT, vil.ResultUnowned)
	bb := f.Entry()
	addr := bb.AllocStack(intT)
	bb.MarkUninitialized(addr, "var")
	bb.Store(bb.IntegerLiteral(intT, 1), addr, "init")
	v := bb.Load(addr, "trivial")
	bb.DeallocStack(addr)
	bb.Return(v)
	wants(t, f, ErrStage)
}
